# Multi-stage build: compile github-mock → distroless runtime image
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY main.go .
RUN go build -o github-mock main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /src/github-mock /github-mock
EXPOSE 8443
ENTRYPOINT ["/github-mock"]
