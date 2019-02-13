FROM golang:alpine As builder

WORKDIR /go/src/app

COPY . .

RUN apk add --no-cache git
RUN go get -d -v ./...
RUN go build -o app

FROM alpine:latest

RUN apk --no-cache add ca-certificates
COPY --from=builder /go/src/app/app /app

EXPOSE 80
ENTRYPOINT ["./app"]
