# --- сборка ---
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY . .

# Кеш модулей и сборки переживает пересборки образа — повторный build занимает секунды.
# CGO_ENABLED=0 даёт статический бинарник: ему не нужна libc в итоговом образе.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- запуск ---
# distroless: в образе только бинарник и сертификаты — ни shell, ни пакетного менеджера,
# меньше поверхность атаки. :nonroot — процесс работает не от root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server

ENV HTTP_ADDR=:8081
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/server"]
