FROM golang:1.20-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/kms ./cmd/kms

FROM scratch
COPY --from=build /out/kms /usr/local/bin/kms
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/kms"]

