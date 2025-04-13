FROM golang:alpine as build
RUN ["apk", "add", "git"]
ADD "https://api.github.com/repos/ypsu/blog/commits?per_page=1" latest_commit_to_invalidate_the_cache
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git", "/blog"]
WORKDIR "/blog/"
RUN ["go", "build", "blog"]

FROM alpine
RUN ["apk", "add", "git"]
ADD "https://api.github.com/repos/ypsu/blog/commits?per_page=1" latest_commit_to_invalidate_the_cache
RUN ["git", "clone", "--depth=1", "--branch=main", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/blog /blog/blog
CMD ["/blog/blog", "-pull", "-api=https://api.iio.ie"]
