FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS build

WORKDIR /src
ARG TARGETOS TARGETARCH
COPY services/api-gateway/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/api-gateway ./cmd

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/api-gateway /usr/local/bin/api-gateway
USER app
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/api-gateway"]
