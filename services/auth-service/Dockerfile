FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS build

WORKDIR /src
ARG TARGETOS TARGETARCH
COPY services/auth-service/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/auth-service ./cmd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
COPY --from=build /out/auth-service /usr/local/bin/auth-service
RUN mkdir -p /data/avatars && chown app:app /data/avatars
USER app
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/auth-service"]
