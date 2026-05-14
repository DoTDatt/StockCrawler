FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /Build

COPY go.mod go.sum ./

RUN go mod download 

COPY . . 

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o main .



FROM alpine:3.23

WORKDIR /APP

COPY --from=builder /Build/main .

CMD ["./main"]