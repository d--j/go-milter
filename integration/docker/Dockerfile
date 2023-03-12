FROM golang:1-bullseye
RUN apt-get update -q \
    && apt-get install -y sudo syslog-ng sasl2-bin libsasl2-2 libsasl2-modules ssl-cert m4 expect tcl-expect cpio  \
    && mkdir /pkgs && cd /pkgs \
    && apt-get download \
       postfix sendmail sendmail-base sendmail-bin sendmail-cf sensible-mda \
       libsigsegv2 maildrop libicu67 libnsl2 \
       courier-authlib libcourier-unicode4 liblockfile-bin liblockfile1 \
       libltdl7 libwrap0 lockfile-progs \
    && dpkg --force-all -i *.deb \
    && rm -rf /pkgs /var/lib/apt/lists/*
RUN mkdir /.cache && chmod 0777 /.cache
RUN git config --global --add safe.directory /usr/src/root
COPY syslog-ng.conf /etc/syslog-ng/syslog-ng.conf
WORKDIR /usr/src/root/integration
CMD ["go", "run", "github.com/d--j/go-milter/integration/runner", "./tests"]
