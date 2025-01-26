# Docker执行
## 构建镜像
根目录执行 

```docker build -t parse-video:v1 .```

## 创建并运行
创建一个名为parse-video的容器，绑定端口，并在容器内执行```/bin/bash```

```docker run -itd -p 8080:8080 --name parse-video parse-video:v1```

# 脚本执行

mac/linux:
```go build -o parse_video```

windows:
```go build -o parse_video.exe```