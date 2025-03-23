FROM golang:1.23  AS build-stage
LABEL authors="wolfgangreithmeier"

ARG upx_version=4.2.2
ARG GOPROXY
ARG TARGETARCH=${TARGETARCH:-amd64}

RUN apt-get update && apt-get install -y --no-install-recommends xz-utils && \
  curl -Ls https://github.com/upx/upx/releases/download/v${upx_version}/upx-${upx_version}-${TARGETARCH}_linux.tar.xz -o - | tar xvJf - -C /tmp && \
  cp /tmp/upx-${upx_version}-${TARGETARCH}_linux/upx /usr/local/bin/ && \
  chmod +x /usr/local/bin/upx && \
  apt-get remove -y xz-utils && \
  rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .
RUN go clean -cache
RUN go mod download
RUN go test ./...

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o openspmreg .
RUN upx --best --lzma openspmreg

RUN addgroup server &&  \
     adduser --ingroup server --uid 19998 --shell /bin/false server && \
     cat /etc/passwd | grep server > /etc/passwd_server

FROM scratch AS production-stage

COPY --from=build-stage  /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build-stage  /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build-stage  /etc/passwd_server /etc/passwd
COPY --from=build-stage  /etc/group /etc/group

WORKDIR /app

COPY --chown=server --from=build-stage /app/openspmreg /app/openspmreg

COPY --chown=server config.yml /app/config.yml
COPY --chown=server server.crt /app/server.crt
COPY --chown=server server.key /app/server.key
COPY --chown=server static  /app/static

EXPOSE 8080

USER server

ENTRYPOINT ["/app/openspmreg", "-v"]