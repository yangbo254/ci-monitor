FROM golang:1.24 AS build-stage

WORKDIR /usr/local/go/src/ci-monitor

COPY go.mod ./
RUN go mod download

COPY fetcher fetcher
COPY logger logger
COPY storage storage
COPY types types
COPY web web
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /ci-monitor

# Run the tests in the container
FROM build-stage AS run-test-stage
RUN go test -v ./...

# Deploy the application binary into a lean image
FROM alpine:3.22 AS build-release-stage

WORKDIR /app
COPY --from=build-stage /ci-monitor /ci-monitor

ENTRYPOINT ["/ci-monitor"]
EXPOSE 8080