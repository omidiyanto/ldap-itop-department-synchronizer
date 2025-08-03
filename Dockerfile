# Build Stage
FROM golang:alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download && go mod tidy
RUN go build -o main .

# Runtime Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
COPY ./data/valid-department-list.yaml /app/data/valid-department-list.yaml
RUN chmod a+x /app/main
CMD ["/app/main"]