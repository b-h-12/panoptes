FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/panoptes .

FROM alpine:3.20
COPY --from=build /out/panoptes /usr/local/bin/panoptes
ENTRYPOINT ["panoptes"]
