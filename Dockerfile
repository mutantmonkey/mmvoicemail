FROM golang:1.18.3-alpine3.16 AS builder

RUN apk --no-cache add libcap

WORKDIR /usr/src/app/

ADD go.mod go.sum /usr/src/app/
ADD *.go /usr/src/app/
RUN go build && setcap 'cap_net_bind_service=+ep' mmvoicemail

FROM alpine:3.16

WORKDIR /usr/src/app

COPY --from=builder /usr/src/app/mmvoicemail /usr/local/bin/mmvoicemail
ADD static /usr/src/app/static/
ADD templates /usr/src/app/templates/

VOLUME ["/etc/mmvoicemail"]

EXPOSE 8080
USER nobody
CMD ["mmvoicemail", "-config", "/etc/mmvoicemail/config.json", "-port", "8080"]
