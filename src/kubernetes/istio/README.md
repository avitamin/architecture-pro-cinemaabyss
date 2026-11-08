# Istio и Circuit Breaker

Учебный sidecar mesh: Istio 1.31.4, Fortio 1.76.0. Фактические результаты и скриншоты — в [шаблоне](../../../Project_template.md#результат-задания-5). Команды выполняются из корня репозитория в Bash; нужны kubectl, Helm, curl, sha256sum, tar и ripgrep (`rg`). Нужен существующий release задания 4 с default values; перед upgrade проверьте `helm get values` и сохраните дополнительные overrides, если они появились.

## Установка

Получите официальный фиксированный дистрибутив, проверьте SHA256 перед распаковкой. Временные файлы оставляются пользователю.

```bash
A=$(mktemp -d /tmp/cinemaabyss-istio.XXXXXX)
curl -fL https://github.com/istio/istio/releases/download/1.31.4/istio-1.31.4-linux-amd64.tar.gz -o "$A/istio.tar.gz"
echo "956421eb4934c10029622b6cd96442c904548d12eadbe874fb1d1a38d5d58ccc  $A/istio.tar.gz" | sha256sum -c -
tar -xzf "$A/istio.tar.gz" -C "$A"
I="$A/istio-1.31.4"
```

В выполненном запуске использован заранее проверенный дистрибутив `.local/tools/istio-1.31.4`. Локальные charts исключают зависимость от наличия версии в Helm index. Образы `istio/pilot:1.31.4`, `istio/proxyv2:1.31.4`, `fortio/fortio:1.76.0` должны быть доступны кластеру.

Все блоки ниже рассчитаны на один shell. В новом shell восстановите `I` (путь к распакованному Istio), создайте `A=$(mktemp -d /tmp/cinemaabyss-istio.XXXXXX)` для новых доказательств и заново определите функции. Они отключают proxy только для конкретной команды и всегда указывают целевой context.

```bash
k() { env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl --context cinemaabyss-task4 --request-timeout=120s "$@"; }
h() { env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy helm --kube-context cinemaabyss-task4 "$@"; }
i() { env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$I/bin/istioctl" --context cinemaabyss-task4 "$@"; }
h list -A
h get values cinemaabyss -n cinemaabyss
k -n cinemaabyss get pods,pvc -o json > "$A/before.json"
i x precheck
h install istio-base "$I/manifests/charts/base" -n istio-system --create-namespace --set defaultRevision=default --wait --timeout 5m
h install istiod "$I/manifests/charts/istio-control/istio-discovery" -n istio-system -f src/kubernetes/istio/istiod-values.yaml --wait --timeout 10m
h install istio-ingressgateway "$I/manifests/charts/gateway" -n istio-system -f src/kubernetes/istio/gateway-values.yaml --wait --timeout 10m
h upgrade cinemaabyss src/kubernetes/helm -n cinemaabyss --reset-values -f src/kubernetes/helm/values-istio.yaml --wait --timeout 10m
k -n cinemaabyss apply -f src/kubernetes/istio/fortio-deploy.yaml
k -n cinemaabyss rollout status deployment/fortio-deploy --timeout=180s
i proxy-status
```

При продолжении установки используйте `status/history` и `upgrade` существующих релизов с теми же values. Не повторяйте install поверх release. Namespace явно задан в namespaced-манифестах. Pod opt-in подключает только monolith, movies, proxy и Fortio. Namespace-wide injection не применяется: events и stateful-зависимости остаются вне mesh. Существующий ingress-nginx обслуживает HTTPS; установленный Istio gateway имеет ClusterIP и не принимает внешний маршрут.

## Проверка

Сначала подтвердите ровно один Ready Pod Fortio. Сохраните UID/restarts клиента и backend: counters нельзя сравнивать после перезапуска Envoy.

```bash
: "${A:?Создайте временный каталог A}"
: "${I:?Укажите путь к Istio 1.31.4}"
k -n cinemaabyss get pods -l app=fortio
F=$(k -n cinemaabyss get pods -l app=fortio -o jsonpath='{.items[0].metadata.name}')
for target in monolith:8080 movies-service:8081; do
  k -n cinemaabyss exec "$F" -c fortio -- fortio curl "http://$target/api/movies"
  k -n cinemaabyss exec "$F" -c fortio -- fortio load -c 1 -qps 2 -n 20 -timeout 3s -loglevel Warning "http://$target/api/movies"
done
i analyze -n cinemaabyss src/kubernetes/circuit-breaker-config.yaml
k -n cinemaabyss apply --dry-run=server -f src/kubernetes/circuit-breaker-config.yaml
k -n cinemaabyss apply -f src/kubernetes/circuit-breaker-config.yaml
i proxy-status
```

Одиночные ответы должны быть непустыми JSON-массивами, спокойные серии — 20/20 HTTP 200. В config dump Fortio **и proxy-service** проверьте thresholds `maxConnections=1`, `maxPendingRequests=1` и `maxRequestsPerConnection=1` (в HTTP protocol options):

```bash
i proxy-config clusters "$F" -n cinemaabyss --fqdn monolith.cinemaabyss.svc.cluster.local --port 8080 -o json
# Повторить для movies-service:8081 и Pod app=proxy-service.
```

Для каждого backend отдельно выполните следующий блок: сначала `target=monolith:8080`, затем `target=movies-service:8081`. Полностью закончите recovery перед вторым сервисом.

```bash
target=monolith:8080
name=${target%:*}; port=${target#*:}
cluster="outbound|$port||$name.cinemaabyss.svc.cluster.local"
start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
for phase in low load recovery; do
  k -n cinemaabyss exec "$F" -c istio-proxy -- pilot-agent request GET stats > "$A/$name-$phase-before.stats"
  if [ "$phase" = load ]; then args='-c 50 -qps 0 -n 500'; else args='-c 1 -qps 2 -n 20'; fi
  k -n cinemaabyss exec "$F" -c fortio -- fortio load $args -timeout 3s -loglevel Warning "http://$target/api/movies" > "$A/$name-$phase.txt" 2>&1
  k -n cinemaabyss exec "$F" -c istio-proxy -- pilot-agent request GET stats > "$A/$name-$phase-after.stats"
  rg -F "$cluster" "$A/$name-$phase-before.stats" "$A/$name-$phase-after.stats" | rg upstream_rq_pending_overflow
  cat "$A/$name-$phase.txt"
done
k -n cinemaabyss logs "$F" -c istio-proxy --since-time="$start" > "$A/$name-access.log"
k -n cinemaabyss get pods,pvc -o json > "$A/after.json"
```

Критерии: HTTP 503 под нагрузкой, положительный delta `upstream_rq_pending_overflow`, `503 UO` (Upstream Overflow — превышение лимита upstream) в access log, затем 20/20 HTTP 200 и delta=0 без снятия политики и рестартов. Отсутствующая метрика не равна нулю. `upstream_rq_pending_total` считает ожидания соединения, а не срабатывания; мгновенный `rq_pending_open=0` после нагрузки нормален. Числа 503 и overflow могут отличаться.

После нагрузки повторите HTTPS через IP task4 (`curl --insecure --noproxy '*' --resolve cinemaabyss.example.com:443:<TASK4_IP>`), переключите Helm `config.moviesMigrationPercent` в 0 и 100 с сохранением overlay, проверьте по 10 GET и Go-логи proxy `upstream=monolith/movies`. Оставьте true/100. Запустите существующий runner `cinemaabyss-api-tests` в Docker network `cinemaabyss-task4` с `--add-host cinemaabyss.example.com:<TASK4_IP>` и аргументом `--environment kubernetes`. Ожидание — 22 requests, 45 assertions, 0 failures. Сравните stateful Pod/PVC UID, restart-count и Bound с baseline; при самом injection events не должен перезапускаться. Переключение общей ConfigMap штатно обновляет events.

## Ограничения

Лимиты 1/1/1 демонстрационные и действуют в каждом вызывающем Envoy. Outlier detection, канареечный rollout и production capacity не проверяются. Одна реплика не даёт HA. `analyze` сообщает IST0102 из-за Pod opt-in и IST0118 для незатронутых events/Kafka/ZooKeeper; это информационные сообщения, HTTP-порты mesh названы явно. HTTP ack events не подтверждает обработку Kafka. Ресурсы оставляются для проверки, удаления и uninstall в этом workflow нет.
