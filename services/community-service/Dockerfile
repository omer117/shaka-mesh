FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS build

WORKDIR /src
ARG TARGETOS TARGETARCH
COPY services/community-service/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/community-service ./cmd

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/community-service /usr/local/bin/community-service
USER app
ENTRYPOINT ["/usr/local/bin/community-service"]
