FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ntfy-adapter .

FROM alpine:3.20
COPY --from=build /out/ntfy-adapter /usr/local/bin/ntfy-adapter
EXPOSE 8080
ENTRYPOINT ["ntfy-adapter"]
