FROM scratch

STOPSIGNAL SIGINT

# add the default configuration file
COPY config/policy-bot.example.yml /secrets/policy-bot.yml

# add static files
COPY ca-certificates.crt /etc/ssl/certs/
COPY mime.types /etc/

# add application files
{{$dist :=  index (InputDistArtifacts "policy-bot" "bin") 0}}
ADD {{$dist}} /

# created by extracting the distribution with ADD
WORKDIR /{{Product}}-{{Version}}

ENTRYPOINT ["bin/linux-amd64/policy-bot"]
CMD ["server", "--config", "/secrets/policy-bot.yml"]
