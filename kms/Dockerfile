FROM golang:1.20-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/progressdb-kms ./cmd/progressdb-kms

FROM scratch
COPY --from=build /out/progressdb-kms /usr/local/bin/progressdb-kms
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/progressdb-kms"]
