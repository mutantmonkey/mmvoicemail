FROM python:3

RUN apt-get update && apt-get install -y libpcre3-dev \
        && pip install uwsgi

WORKDIR /usr/src/app

COPY requirements.in /usr/src/app/
COPY requirements.txt /usr/src/app/
RUN pip install --no-cache-dir -r requirements.txt

ADD mmvoicemail /usr/src/app/mmvoicemail/
COPY uwsgi.ini /usr/src/app/

VOLUME ["/etc/mmvoicemail"]

EXPOSE 8080
USER nobody
CMD ["uwsgi", "--ini", "uwsgi.ini:docker"]
