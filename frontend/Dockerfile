# First stage: build the Go binary
FROM --platform=$BUILDPLATFORM docker.io/golang:1.22.1 AS go-builder

WORKDIR /app

RUN \
  --mount=type=bind,source=./go.mod,target=/app/go.mod \
  --mount=type=bind,source=./go.sum,target=/app/go.sum \
  go mod download

COPY . .

ARG TARGETARCH
RUN \
  GOOS=linux \
  GOARCH=$TARGETARCH \
  CGO_ENABLED=0 \
  go build -a -installsuffix cgo -o ./threat-frontend .

# Second stage: build the React frontend
FROM --platform=$BUILDPLATFORM node:16 AS react-builder

WORKDIR /app

COPY ./react/package.json ./react/package-lock.json ./
RUN npm install

# Copy React app source
COPY ./react/ ./

# Amend baseurl to string
RUN \
  sed -i -e "s/let baseurl.*/let baseurl = ''/" ./src/Dashboard2.js \
  && \
  npm run build # Install dependencies and build static files


# Third stage: assemble the final image
FROM scratch

EXPOSE 8080

# Define DOCROOT env variable
ENV DOCROOT=/var/www/html

LABEL maintainer="kin.wai.koo@gmail.com"

# Copy the Go binary from the first stage
COPY --from=go-builder --chown=1000:0 --chmod=775 /app/threat-frontend /usr/local/bin/threat-frontend

# Copy the built React app from the second stage
COPY --from=react-builder /app/build $DOCROOT

CMD ["/usr/local/bin/threat-frontend"]
