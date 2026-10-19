FROM golang:1.24 AS builder

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser

WORKDIR /app
COPY --from=builder /app/server /app/server
COPY --from=builder /app/config /app/config

USER 10001
ENV GIN_MODE=release
EXPOSE 3000

ENTRYPOINT ["/app/server"]
