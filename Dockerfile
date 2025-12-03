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

# 设置时区
ENV TZ Asia/Shanghai

# 安装 tzdata，复制时区文件，设置时区文件，最后删除 tzdata 包
RUN apk --no-cache add ca-certificates \
    && apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo ${TZ} > /etc/timezone \
    && apk del tzdata

WORKDIR /app
COPY --from=build-stage /ci-monitor /ci-monitor

ENTRYPOINT ["/ci-monitor"]
EXPOSE 8080