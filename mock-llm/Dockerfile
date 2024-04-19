FROM --platform=$BUILDPLATFORM docker.io/golang:1.22.1 AS builder

ARG TARGETARCH

WORKDIR /app

RUN \
  --mount=type=bind,source=./go.mod,target=/app/go.mod \
  --mount=type=bind,source=./go.sum,target=/app/go.sum \
  go mod download

COPY . .

RUN \
  GOOS=linux \
  GOARCH=$TARGETARCH \
  CGO_ENABLED=0 \
  go build -a -installsuffix cgo -o ./mock-llm .

FROM scratch

EXPOSE 8080

LABEL maintainer="kin.wai.koo@gmail.com"

COPY --from=builder --chown=1000:0 --chmod=775 /app/mock-llm /usr/local/bin/mock-llm

CMD ["/usr/local/bin/mock-llm"]