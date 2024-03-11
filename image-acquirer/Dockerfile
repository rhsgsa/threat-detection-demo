ARG BASE_IMAGE=nvcr.io/nvidia/cuda:12.3.1-devel-ubi9
FROM $BASE_IMAGE

CMD ["python3", "./app.py"]

ENV \
  LD_LIBRARY_PATH=/usr/local/cuda/lib64:/usr/local/lib/python3.9/site-packages/nvidia/cudnn/lib \
  CUDA_DIR=/usr/local/cuda \
  CUDA_PATH=/usr/local/cuda \
  CUDA_INSTALL_DIR=/usr/local/cuda \
  CUDA_HOME=/usr/local/cuda \
  NVIDIA_VISIBLE_DEVICES=all \
  HOME=/app \
  CAMERA=/dev/video0

RUN \
  yum update -y \
  && \
  yum install -y python3-pip mesa-libGL libjpeg libpng libjpeg-devel libpng-devel python3-devel \
  && \
  yum clean all

WORKDIR /app
EXPOSE 8080
COPY --chown=:0 ./app/requirements.txt /app/

RUN pip3 install --no-cache-dir -r requirements.txt

COPY --chown=:0 best.pt /app/
COPY --chown=:0 ./app/ /app/
RUN chmod -R g=u /app

COPY app/ /app
