FROM golang:1.27.2-alpine AS build

WORKDIR /src
COPY services/beach-service/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/beach-service ./cmd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
COPY --from=build /out/beach-service /usr/local/bin/beach-service
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/beach-service"]
