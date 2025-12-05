# SSL 证书配置说明

## 推荐方案：在 Nginx 层配置 SSL

由于本项目采用 302 重定向架构，**推荐在 Nginx 节点上配置 SSL**，而不是在 go-emby2openlist 服务上配置。

这样做的好处：
- 更好的性能（Nginx 处理 SSL 更高效）
- 更灵活的证书管理
- 减少代理服务器负载

## 可选方案：在代理服务上配置 SSL

如果你确实需要在 go-emby2openlist 服务上启用 HTTPS，请按以下步骤操作：

1. 将 `.crt` 和 `.key` 文件放在此目录下
2. 在 `config.yml` 中配置：

```yaml
ssl:
  enable: true
  key: your-cert.key
  crt: your-cert.crt
  single-port: false  # false: 同时监听 8094(HTTPS) 和 8095(HTTP)
                      # true: 只监听 8094(HTTPS)
```

3. 使用 Docker 时，需要挂载此目录：

```yaml
volumes:
  - ./ssl:/app/ssl
```

## 注意事项

- **不要提交证书文件到 Git 仓库**（已在 .gitignore 中配置）
- 证书文件权限建议设置为 600 或 400
- 定期检查证书过期时间