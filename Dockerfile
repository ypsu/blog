FROM alpine as build
RUN ["apk", "add", "git", "go"]
ADD https://api.github.com/repos/ypsu/blog/git/refs/heads/master version.json
RUN ["git", "clone", "--depth=1", "--branch=master", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
RUN ["go", "build", "blog"]

FROM alpine
RUN ["apk", "add", "git"]
ADD https://api.github.com/repos/ypsu/blog/git/refs/heads/master version.json
RUN ["git", "clone", "--depth=1", "--branch=master", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/blog /blog/blog
CMD ["/blog/blog", "-pull", "-api=https://api.iio.ie"]
