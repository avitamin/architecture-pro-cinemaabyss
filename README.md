# Архитектура микросервисов CinemaAbyss

## Обзор.
 В проекте реализована следующая функциональность:

- Извлечение микросервисов с использованием паттерна Strangler Fig
- Развертывание в Kubernetes для оркестрации и масштабирования
- API Gateway для унифицированного доступа к сервисам
- Архитектура, управляемая событиями, с использованием Kafka
- CI/CD Pipeline с GitHub Actions

## Компоненты

### Монолит
Исходное монолитное приложение обрабатывает:

- Управление пользователями
- Метаданные фильмов
- Платежи
- Подписки

Сервис расположен в src/monolith/.

### Микросервисы

#### Movies Service
Извлечен из монолита, обрабатывает всю функциональность, связанную с фильмами:

- Метаданные фильмов
- Рейтинги
- Жанры

Расположен в src/microservices/movies/.

#### Events Service
Обрабатывает коммуникацию между сервисами на основе событий с использованием Kafka:

- События фильмов (просмотр, оценка, добавление)
- События пользователей (регистрация, вход)
- События платежей (успешные, неудачные)

Расположен в src/microservices/events/.

#### Proxy Service (API Gateway)
Реализует функционал для постепенного перехода от монолита к микросервисам:

- Маршрутизация запросов между монолитом и микросервисами
- Поддержка постепенного перехода с процентной маршрутизацией
- Действует как фасад для всей системы

Расположен в src/microservices/proxy/.

## Инфраструктура

### Kubernetes
Манифесты Kubernetes для развертывания всех компонентов расположены в src/kubernetes/.

### Helm Charts
Charts Helm для упрощения развертывания и управления:

Расположены в src/kubernetes/helm/.

### Kafka
Расположено в src/kubernetes/kafka/.

### CI/CD Pipeline
GitHub Actions для непрерывной интеграции и развертывания:

- Сборка и тестирование микросервисов
- Сборка и выгрузка Docker-образов

Расположены в .github/workflows/.


## Детали реализации

#### Паттерн Strangler Fig

Реализован через proxy-сервис, который выступает в роли фасада перед монолитом и микросервисами. Он маршрутизирует трафик на основе конфигурации:

- При включенном фиче-флаге маршрутизирует определенный процент трафика в микросервис
- При отключенном маршрутизирует весь трафик для определенного домена в соответствующий микросервис

Это позволяет осуществлять контролируемый постепенный переход без нарушения работы пользователей.

## Deployment Instructions

### Local Development with Docker Compose

1. Необходимо, чтобы был установлен docker и docker-compose

2. Запускаем сервисы с помощью Docker Compose:
   ```bash
   docker-compose up -d
   ```

После запуска сервисы доступны:
- Monolith: http://localhost:8080
- Movies Service: http://localhost:8081
- Events Service: http://localhost:8082
- API Gateway (Proxy): http://localhost:8000
- Kafka UI: http://localhost:8090

3. Останавливаем сервисы:
   ```bash
   docker compose stop
   ```

4. После внесения изменений рестартим:

   ```bash
   docker-compose build
   docker-compose up -d
   ```

### Kubernetes Deployment

#### Требования

- Kubernetes cluster (v1.19+)
- Helm (v3.2.0+)
- kubectl

#### Развертывание

1. Создайте namespace:
```bash
kubectl apply -f src/kubernetes/namespace.yaml
```
2. Разверните Kafka:
```bash
kubectl apply -f src/kubernetes/kafka/kafka.yaml
```
3. Разверните базу данных:
```bash
kubectl apply -f src/kubernetes/postgres.yaml
```
4. Разверните монолит:
```bash
kubectl apply -f src/kubernetes/monolith.yaml
```
5.Разверните микросервисы:
```bash
kubectl apply -f src/kubernetes/movies-service.yaml
kubectl apply -f src/kubernetes/events-service.yaml
```
6. Разверните прокси-сервис:
```bash
kubectl apply -f src/kubernetes/proxy-service.yaml
```

### Развертывание через CI/CD
Проект включает GitHub Actions для CI/CD:

- Сборка и тестирование: Автоматически собирает и тестирует код при пуше или пул-реквесте.
- Сборка Docker и выгрузка: Создает Docker-образы и выгружает их в GitHub Container Registry.

Чтобы использовать пайплайн CI/CD:

1. Создайте форк или клонируйте этот репозиторий в свой аккаунт GitHub.
2. Отправьте изменения в основную ветку для запуска пайплайна CI/CD.
3. Выполните ручное или автоматическое развертывание (Helm) в локальной среде

## Тестирование API с Postman
Проект включает комплексный набор тестов Postman, которые можно запускать из командной строки с помощью Newman. 

Тесты проверяют базовую функциональность всех сервисов в архитектуре.

Покрытие тестами
-  сервис: Пользователи, Фильмы, Платежи, Подписки
- Микросервис фильмов: Проверка работоспособности, Операции с фильмами
- Микросервис событий: Проверка работоспособности, Публикация событий
- Прокси-сервис: Проверка работоспособности, Проксирование запросов

### Запуск тестов

#### Предварительные требования

- Node.js (v14 или выше)
- npm (v6 или выше)
- Newman (установлен через npm)

#### Установка
1. Перейдите в директорию тестов
```bash
cd tests/postman
```
2. Установите зависимости
```bash
npm install
```
3. Запуск тестов локально
```bash
npm run test:local
```
или
```bash
npm run test:docker
```
4. Запуск тестов с помощью shell-скрипта
1. Сделайте скрипт исполняемым
chmod +x run-tests.sh

2. Запустите все тесты
```bash
./run-tests.sh -e local
```
или
```bash
./run-tests.sh -d -e docker
```

### Тестирование деплоя руками
1. Тестирование с Docker Compose

   Отправьте запросы к API Gateway:
   ```bash
   curl http://localhost:8000/api/movies
   ```
2. Протестируйте постепенный переход, изменив переменную окружения MOVIES_MIGRATION_PERCENT в файле docker-compose.yml.

3. Проверьте топики Kafka и сообщения через Kafka UI по адресу http://localhost:8090
## Реализация задания 2

Go proxy и events реализованы. Proxy распределяет каждый запрос каталога независимо; `GRADUAL_MIGRATION=false` направляет весь каталог в movies. Откат: `true` и `0`. После изменения окружения нужен `docker compose up -d --no-deps --force-recreate proxy-service`. Конфигурация читается при старте. При отказе выбранного upstream возвращается 502/504, повторов и fallback нет.

Events валидирует JSON и публикует envelope синхронно с `RequireAll`; 201 содержит координаты подтверждённой записи. Consumer группы `cinemaabyss-events` пишет результат в stdout, затем сохраняет offset. Повторные строки после сбоя допустимы. Health подтверждает только работу HTTP. Общая БД, один брокер и отсутствие идемпотентности HTTP сохранены для учебного MVP; Saga и Outbox здесь не реализованы.

Запуск: `docker compose up -d --build`. Проверки: `cd tests/postman && npm install && npm run test:local`. Остановка: `docker compose stop`.

Фактическая приёмка и ограничения хоста: [результат задания 2](Project_template.md#результат-задания-2). Решения: [миграция трафика](docs/architecture/decisions/011-movies-traffic-migration.md), [доставка событий](docs/architecture/decisions/012-events-mvp-delivery.md). Результаты Kubernetes и Helm приведены в [задании 3](Project_template.md#результат-задания-3) и [задании 4](Project_template.md#результат-задания-4). Инструкция: [Helm README](src/kubernetes/helm/README.md).

Задание 5: [результаты Circuit Breaker и реальные скриншоты](Project_template.md#результат-задания-5), [инструкция Istio/Fortio](src/kubernetes/istio/README.md).
