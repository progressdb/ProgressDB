FROM golang:1.20-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/minikms ./cmd/minikms

FROM scratch
COPY --from=build /out/minikms /usr/local/bin/minikms
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/minikms"]

