FROM golang:1.23  AS build-stage
LABEL authors="wolfgangreithmeier"

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o openspmreg .

RUN addgroup server &&  \
     adduser --ingroup server --uid 19998 --shell /bin/false server && \
     cat /etc/passwd | grep server > /etc/passwd_server

FROM scratch AS production-stage

COPY --from=build-stage  /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build-stage  /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build-stage  /etc/passwd /etc/passwd
COPY --from=build-stage  /etc/group /etc/group

WORKDIR /app

COPY --chown=server --from=build-stage /app/openspmreg /app/openspmreg

COPY --chown=server .docker/config.yml /app/config.yml
COPY --chown=server .docker/certs /app/certs

EXPOSE 8080

USER server

ENTRYPOINT ["/app/openspmreg", "-tls=true", "-v"]