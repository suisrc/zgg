FROM golang:1.25-alpine3.23 AS build_deps

RUN apk add --no-cache git

WORKDIR /opt

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN CGO_ENABLED=0 go build -o zgg -ldflags '-w -extldflags "-static"' ./app

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /opt
COPY --from=build /opt/zgg /opt/zgg

ENTRYPOINT ["./zgg"]
