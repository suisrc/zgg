FROM busybox:1.37.0
CMD ["sh", "-c", "wget -q -O - \"$TAR_URL\" | tar -xvC /data"]