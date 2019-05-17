FROM fedora:29

COPY ./bin/gcs-provisioner /usr/bin/

ENTRYPOINT ["/usr/bin/gcs-provisioner"]
CMD ["-v=2", "-alsologtostderr"]