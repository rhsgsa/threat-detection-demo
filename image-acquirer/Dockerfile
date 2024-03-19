# amd64
FROM nvcr.io/nvidia/cuda:12.3.1-devel-ubi9 AS base-amd64

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


# arm64
FROM nvcr.io/nvidia/l4t-pytorch:r35.2.1-pth2.0-py3 AS base-arm64

RUN \
  apt update -y \
  && \
  apt install -y curl \
  && \
  apt clean all \
  && \
  pip3 install --upgrade pip \
  && \
  rm -f /usr/local/lib/python3.8/dist-packages/cv2 \
  && \
  mv /usr/lib/python3.8/dist-packages/* /usr/local/lib/python3.8/dist-packages/ \
  && \
  rm -rf /usr/lib/python3.8/dist-packages \
  && \
  ln -s /usr/lib/local/lib/python3.8/dist-packages /usr/lib/python3.8/dist-packages \
  && \
  pip3 uninstall -y opencv-python \
  && \
  rm -rf /usr/local/lib/python3.8/dist-packages/cv2 \
  && \
  pip3 install -U numpy \
  && \
  pip3 install opencv-python==4.8.1.78


ARG TARGETARCH
FROM base-$TARGETARCH

CMD ["python3", "./app.py"]

ENV \
  HOME=/app \
  CAMERA=/dev/video0 \
  MODEL=$MODEL_NAME

WORKDIR /app
EXPOSE 8080
COPY --chown=:0 ./app/requirements.txt /app/

RUN pip3 install --no-cache-dir -r requirements.txt

ARG MODEL_URL MODEL_NAME
RUN curl -LO $MODEL_URL

COPY --chown=:0 ./app/ /app/
RUN chmod -R g=u /app

COPY app/ /app
