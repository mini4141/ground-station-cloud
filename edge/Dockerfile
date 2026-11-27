FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ ./edge/agent ./edge/sim-modem ./edge/sim-rotator

FROM alpine:3.22
RUN apk add --no-cache socat ca-certificates
COPY --from=build /out/agent /out/sim-modem /out/sim-rotator /usr/local/bin/
COPY edge/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
