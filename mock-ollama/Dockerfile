FROM --platform=$BUILDPLATFORM docker.io/golang:1.22.1 AS builder

ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod download

COPY . /app/

RUN \
  GOOS=linux \
  GOARCH=$TARGETARCH \
  CGO_ENABLED=0 \
  go build -a -installsuffix cgo -o ./mock-ollama .

FROM scratch

EXPOSE 8080

LABEL maintainer="kin.wai.koo@gmail.com"

COPY --from=builder --chown=1000:0 --chmod=775 /app/mock-ollama /usr/local/bin/mock-ollama

CMD ["/usr/local/bin/mock-ollama"]