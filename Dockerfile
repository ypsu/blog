FROM alpine as build
RUN ["apk", "add", "git", "go"]
ADD https://api.github.com/repos/ypsu/blog/git/refs/heads/master version.json
RUN ["git", "clone", "--depth=1", "--branch=master", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
RUN ["go", "build", "serve.go"]

FROM alpine
RUN ["apk", "add", "git"]
RUN ["git", "clone", "--depth=1", "--branch=master", "https://github.com/ypsu/blog.git"]
WORKDIR "/blog/"
COPY --from=build /blog/serve /blog/serve
COPY --from=flyio/litefs /usr/local/bin/litefs /bin/litefs
CMD git pull && /blog/serve -postpath=/blog/docs
