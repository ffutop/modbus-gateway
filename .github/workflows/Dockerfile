# ---- Builder Stage ----
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /modbus-gateway .

# ---- Final Stage ----
FROM alpine:latest
COPY --from=builder /modbus-gateway /usr/bin/modbus-gateway
ENTRYPOINT [ "/usr/bin/modbus-gateway", "-c", "/etc/modbusgw/modbusgw.conf" ]