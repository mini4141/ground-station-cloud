# One image per service (ARG CMD = api | ingest | scheduler). Runs as an AWS Lambda container image when
# AWS_LAMBDA_RUNTIME_API is set, and as a plain HTTP server / poller otherwise (kind, local).
#
# Lambda container images cannot attach layers, so scripts/deploy.sh downloads the ADOT collector layer and
# passes its extracted files as the named build context "adot"; without it the stage below is empty (kind/local).
FROM golang:1.26-alpine AS build
ARG CMD=api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o /out/main ./cmd/${CMD}

FROM scratch AS adot

FROM public.ecr.aws/lambda/provided:al2023
COPY --from=adot / /opt/
RUN if [ -d /opt/extensions ]; then chmod -R +x /opt/extensions; fi
COPY --from=build /out/main ./main
ENTRYPOINT ["./main"]
