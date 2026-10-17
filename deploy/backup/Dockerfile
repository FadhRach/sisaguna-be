FROM postgres:16-alpine
RUN apk add --no-cache rclone tzdata
COPY entrypoint.sh backup.sh restore-test.sh restore.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/backup.sh /usr/local/bin/restore-test.sh /usr/local/bin/restore.sh
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["run"]
