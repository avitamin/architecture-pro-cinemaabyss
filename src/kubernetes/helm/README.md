# Helm-развёртывание CinemaAbyss

Chart находится в `src/kubernetes/helm`; команды ниже выполняются из корня репозитория. Нужны Helm 3, kubectl, доступный Kubernetes, ingress-nginx с IngressClass `nginx` и динамический storage provisioner. Проверенные версии и результаты приведены в [отчёте задания 4](../../../Project_template.md#результат-задания-4).

В одном namespace поддерживается один релиз: имена сервисов стабильны. Для установки используйте пустой namespace. Существующие raw-ресурсы вызывают конфликт владения с Helm; выбирайте отдельное окружение, сохраняя прежние ресурсы и данные.

```bash
helm lint src/kubernetes/helm --strict
helm --kube-context cinemaabyss-task4 install cinemaabyss src/kubernetes/helm \
  --namespace cinemaabyss --create-namespace --wait --timeout 10m
kubectl --context cinemaabyss-task4 -n cinemaabyss get pods,svc,ingress,pvc
```

Четыре прикладных образа по умолчанию берутся из `ghcr.io/avitamin/architecture-pro-cinemaabyss/` с фиксированным тегом `sha-27dac03`. Их параметры: `monolith.image`, `moviesService.image`, `eventsService.image`, `proxyService.image` (`repository`, `tag`, `pullPolicy`). Не заменяйте фиксированный тег на `latest` для воспроизводимой установки.

`imagePullSecrets.dockerconfigjson` содержит base64 Docker config JSON; default `eyJhdXRocyI6e319` означает `{"auths":{}}` и подходит для этих публичных образов. Приватные credentials передавайте непубликуемым values-файлом через `-f`. Не сохраняйте PAT в Git. `database.password` также ожидает base64; значение по умолчанию — учебное, для другого окружения задайте собственное до первой установки.

PostgreSQL init SQL выполняется только на свежем volume. Обновление ConfigMap не мигрирует существующую БД. По умолчанию три PVC сохраняют PostgreSQL, Kafka и ZooKeeper; отключение persistence даёт `emptyDir` и потерю данных при пересоздании Pod.

Ingress обслуживает `cinemaabyss.example.com`: `/` направляется на `proxy-service:80`, `/api/events` — на `events-service:8082`. При изменении `*.service.port` согласованно меняйте `ingress.hosts[].paths[].port`. `ingress.tls` принимает список hosts и необязательный `secretName` существующего TLS Secret. По умолчанию используется учебный default certificate ingress controller; доверенная публичная TLS-цепочка не настроена.

Для локального профиля с IP `192.168.194.2`:

```bash
curl --insecure --noproxy '*' \
  --resolve cinemaabyss.example.com:443:192.168.194.2 \
  https://cinemaabyss.example.com/api/movies
```

`--insecure` допустим здесь для учебного сертификата. В другом окружении используйте его фактический IP и доверенный TLS. Запись hosts старого профиля не подтверждает новый deploy.

Изменение конфигурации вызывает rollout четырёх приложений через checksum ConfigMap:

```bash
helm --kube-context cinemaabyss-task4 upgrade cinemaabyss src/kubernetes/helm \
  --namespace cinemaabyss --reset-values \
  --set-string config.moviesMigrationPercent=0 --wait --timeout 10m
# Возврат defaults true/100; для собственных настроек повторно передайте -f.
helm --kube-context cinemaabyss-task4 upgrade cinemaabyss src/kubernetes/helm \
  --namespace cinemaabyss --reset-values --wait --timeout 10m
```

Одна реплика и один Kafka broker не обеспечивают HA или непрерывную доступность при rollout. HTTP 201 events означает ack публикации; обработку проверяют отдельно по `event_processed` и consumer lag.

## Опциональный Istio

`istio.enabled` по умолчанию `false`. После установки control plane передавайте `-f src/kubernetes/helm/values-istio.yaml` при **каждом** upgrade из корня репозитория: overlay включает sidecar у monolith, movies и proxy. Без overlay chart пригоден для работы без Istio. Политики имеют отдельного владельца и применяются raw-манифестом. [Установка, Fortio и ограничения](../istio/README.md).
