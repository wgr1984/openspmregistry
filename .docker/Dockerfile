FROM golang:1.23  AS build-stage
LABEL authors="wolfgangreithmeier"

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o openspmreg .

FROM alpine:latest AS production-stage

WORKDIR /app

COPY --from=build-stage /app/openspmreg /app/openspmreg

COPY .docker/config.yml /app/config.yml
COPY .docker/certs /app/certs

EXPOSE 8080

RUN addgroup -S server && adduser -S server -G server && chown -R server:server /app

ENTRYPOINT ["/app/openspmreg", "-tls=false", "-v"]