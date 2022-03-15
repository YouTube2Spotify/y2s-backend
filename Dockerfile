#build binary
FROM golang:alpine AS build-env

RUN apk add --no-cache git

ADD . /src

RUN cd /src && go build -o main


# run build
FROM alpine

RUN apk add --update --no-cache ca-certificates ffmpeg

WORKDIR /app

COPY --from=build-env /src/main /app
COPY .env /app

EXPOSE 3000

CMD ./main