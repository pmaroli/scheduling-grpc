FROM golang:alpine
RUN mkdir /app
ADD . /app/
WORKDIR /app

# Hot reloading for development
RUN go get github.com/githubnemo/CompileDaemon

ENTRYPOINT CompileDaemon --build="go build main.go" --command="./main"