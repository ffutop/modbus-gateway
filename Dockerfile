FROM alpine:latest
COPY modbus-gateway /usr/bin/modbus-gateway
ENTRYPOINT [ "/usr/bin/modbus-gateway", "-c", "/etc/modbusgw/modbusgw.conf" ]