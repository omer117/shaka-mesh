FROM golang:1.27.2-alpine AS build

WORKDIR /src
COPY services/notification-service/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/notification-service ./cmd

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/notification-service /usr/local/bin/notification-service
USER app
ENTRYPOINT ["/usr/local/bin/notification-service"]
