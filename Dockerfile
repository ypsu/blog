FROM alpine as build
RUN ["apk", "add", "git", "go"]
RUN ["git", "clone", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
RUN ["go", "build", "serve.go"]

FROM alpine
RUN ["apk", "add", "git"]
RUN ["git", "clone", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/serve /blog/serve
COPY --from=flyio/litefs /usr/local/bin/litefs /bin/litefs
CMD git pull && /blog/serve -postpath=/blog/docs
