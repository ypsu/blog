FROM golang:alpine as build
RUN ["apk", "add", "git"]
ADD "https://uuid.rocks/short" /tmp/cachebuster.txt
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git", "/blog"]
WORKDIR "/blog/"
RUN ["go", "build", "blog"]

FROM alpine
RUN ["apk", "add", "git"]
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/blog /blog/blog
CMD ["/blog/blog", "-pull", "-api=https://api.iio.ie"]
