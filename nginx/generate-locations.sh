#!/bin/bash
# Nginx 多目录 location 配置生成器
# 使用方法：./generate-locations.sh > locations.conf

cat << 'EOF'
# ============================================
# 多目录 location 配置
# 根据 config.yml 中的 emby2nginx 映射自动生成
# ============================================

# 目录映射示例：
# emby2nginx:
#   - /media/data:/video/data       -> Nginx: location /video/data { alias /mnt/google/; }
#   - /media/data1:/video/data1     -> Nginx: location /video/data1 { alias /mnt/google1/; }

EOF

# 定义需要映射的目录
declare -A MAPPINGS=(
    ["/video/data"]="/mnt/google"
    ["/video/data1"]="/mnt/google1"
    ["/video/data2"]="/mnt/google2"
    ["/video/data3"]="/mnt/google3"
    ["/video/data4"]="/mnt/google4"
    ["/video/data_2"]="/mnt/google_2"
    ["/video/data_3_oumeiguochan"]="/mnt/google_3_oumeiguochan"
)

# 生成每个 location 块
for nginx_path in "${!MAPPINGS[@]}"; do
    host_path="${MAPPINGS[$nginx_path]}"

    cat << EOF
# ${nginx_path} -> ${host_path}
location ${nginx_path} {
    alias ${host_path}/;

    # OPTIONS 预检请求
    if (\$request_method = 'OPTIONS') {
        add_header 'Access-Control-Allow-Origin' '*';
        add_header 'Access-Control-Allow-Methods' 'GET, HEAD, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'Range, Origin, Accept, Content-Type, Authorization, X-Emby-Token, X-Emby-Authorization';
        add_header 'Access-Control-Max-Age' 86400;
        add_header 'Content-Type' 'text/plain charset=UTF-8';
        add_header 'Content-Length' 0;
        return 204;
    }

    # Range 请求支持
    add_header 'Accept-Ranges' bytes always;

    # CORS Headers
    add_header 'Access-Control-Allow-Origin' '*' always;
    add_header 'Access-Control-Allow-Methods' 'GET, HEAD, OPTIONS' always;

    # 文件类型
    types {
        video/mp4       mp4;
        video/x-matroska mkv;
        video/webm      webm;
        video/x-msvideo avi;
        video/quicktime mov;
        audio/mpeg      mp3;
        audio/x-flac    flac;
        audio/x-wav     wav;
        text/vtt        vtt;
        application/x-subrip srt;
    }

    # 性能优化
    gzip off;
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    directio 512;
    output_buffers 1 1m;

    # 超时设置
    send_timeout 3600s;
    keepalive_timeout 3600s;
}

EOF
done

cat << 'EOF'
# 生成完毕
# 请将上面的配置复制到 /etc/nginx/conf.d/video.conf 的 server 块中
EOF
