FROM --platform=$BUILDPLATFORM docker.io/golang:1.22.1 AS builder

ARG TARGETARCH

COPY . /app/

RUN \
  cd /app/ \
  && \
  GOOS=linux GOARCH=$TARGETARCH CGO_ENABLED=0 go build -a -installsuffix cgo -o ./threat-frontend .

FROM scratch

EXPOSE 8080

LABEL maintainer="kin.wai.koo@gmail.com"

COPY --from=builder --chown=1000:0 --chmod=775 /app/threat-frontend /usr/local/bin/threat-frontend

CMD ["/usr/local/bin/threat-frontend"]