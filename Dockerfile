FROM golang:1.25-alpine3.23 AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN CGO_ENABLED=0 go build -o zgg -ldflags '-w -extldflags "-static"' .

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /workspace/zgg /usr/local/bin/zgg

ENTRYPOINT ["zgg"]
