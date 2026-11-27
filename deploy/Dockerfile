# One image per service (ARG CMD = api | ingest | scheduler). Runs as an AWS Lambda container image when
# AWS_LAMBDA_RUNTIME_API is set, and as a plain HTTP server / poller otherwise (kind, local).
FROM golang:1.26-alpine AS build
ARG CMD=api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o /out/main ./cmd/${CMD}

FROM public.ecr.aws/lambda/provided:al2023
COPY --from=build /out/main ./main
ENTRYPOINT ["./main"]
