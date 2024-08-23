FROM alpine as build
RUN ["apk", "add", "git", "go"]
COPY .git/refs/heads/main version
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
RUN ["go", "build", "blog"]

FROM alpine
RUN ["apk", "add", "git"]
COPY .git/refs/heads/main version
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/blog /blog/blog
CMD ["/blog/blog", "-pull", "-api=https://api.iio.ie"]
