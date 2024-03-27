# First stage: build the Go binary
FROM --platform=$BUILDPLATFORM docker.io/golang:1.22.1 AS go-builder

ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN \
  GOOS=linux \
  GOARCH=$TARGETARCH \
  CGO_ENABLED=0 \
  go build -a -installsuffix cgo -o ./threat-frontend .

# Second stage: build the React frontend
FROM node:16 AS react-builder

WORKDIR /app

# Copy React app source
COPY ./react ./

# Amend baseurl to string
RUN sed -i -e "s/let baseurl.*/let baseurl = ''/" ./src/Dashboard.js

# Install dependencies and build static files
RUN npm install && npm run build

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
