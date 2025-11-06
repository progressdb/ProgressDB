FROM golang:1.21-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/prgkms ./cmd/prgkms

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=build /out/prgkms /usr/local/bin/prgkms
RUN chmod +x /usr/local/bin/prgkms
EXPOSE 6820
ENTRYPOINT ["/usr/local/bin/prgkms"]
