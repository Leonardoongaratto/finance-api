FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/finance-api ./cmd/finance-api

FROM alpine:3
RUN addgroup -S -g 10001 finance && adduser -S -u 10001 -G finance finance \
    && mkdir /data && chown finance:finance /data
COPY --from=build /out/finance-api /usr/local/bin/finance-api
USER finance
ENV FINANCE_ADDR=0.0.0.0:8080 \
    FINANCE_DB=/data/finance.db
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["finance-api"]
