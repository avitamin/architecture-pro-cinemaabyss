FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o /service .
FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY --from=builder /service /service
EXPOSE 8000
CMD ["/service"]
