FROM ubuntu:22.04


#install dependencies
RUN apt update && apt install -y \
   openmpi-bin \
   libopenmpi-dev \
   openmpi-common \
   mpich \
   pkg-config \
   make \
   git \
   pip \
   iputils-ping \
   telnet \
   curl \
   jq \
   libaio-dev 

RUN mkdir -p /root/git

#install ior
RUN cd /root/git && git clone https://github.com/hpc/ior.git && cd ior \
    && git checkout remotes/origin/4.0 \
    && ./bootstrap && ./configure --prefix="$HOME" && make && make install

#install fio from source
RUN cd /root/git && git clone --depth=1 https://github.com/axboe/fio.git && cd fio \
    && ./configure --prefix="$HOME" && make && make install

COPY scripts /root/

COPY benchmark_start.sh /root/
RUN chmod +x /root/*.sh /root/bin/*

#FIO server
EXPOSE 8765

CMD ["/root/bin/fio", "--server"]
