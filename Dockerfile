FROM ubuntu:22.04

ENV LANG=zh_CN.UTF-8 TZ=Asia/Shanghai

WORKDIR /app

COPY set-resource /usr/local/bin/set-resource

RUN chmod +x /usr/local/bin/set-resource

ENTRYPOINT ["set-resource"]
