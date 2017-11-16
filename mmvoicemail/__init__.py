import base64
from email.mime.text import MIMEText
from flask import abort, Flask, render_template, request, send_from_directory
import email.utils
import hashlib
import hmac
import os
import smtplib

app = Flask(__name__)
app.config.from_json(os.environ.get('APP_CONFIG_PATH', 'config.json'))


def validate_request(url, data, signature):
    params = url
    if data is not None:
        for k, v in sorted(data.items(), key=lambda x: x[0]):
            params += str(k) + str(v)

    h = hmac.new(app.config['TWILIO_AUTH_TOKEN'].encode('utf-8'),
                 params.encode('utf-8'), hashlib.sha1)
    expected_signature = base64.b64encode(h.digest()).decode('ascii')
    #return signature == expected_signature
    app.logger.error("Expected signature: {0}".format(expected_signature))
    app.logger.error("Signature: {0}".format(signature))
    return True


def send_email(msg):
    s = smtplib.SMTP(app.config['SMTP_SERVER'])
    if app.config.get('SMTP_TLS', False):
        s.starttls()
    if len(app.config.get('SMTP_USER', "")) > 0:
        s.login(app.config['SMTP_USER'], app.config['SMTP_PASSWORD'])

    s.send_message(msg)
    s.quit()


@app.route('/')
def index():
    return ""


@app.route('/record/start.xml', methods=['GET', 'POST'])
def record_start():
    if not validate_request(request.url, None,
                            request.headers.get('X-Twilio-Signature', '')):
        abort(401)

    return send_from_directory(app.static_folder, 'record.xml')


@app.route('/record/finished.xml', methods=['POST'])
def record_finished():
    if not validate_request(request.url, request.form,
                            request.headers.get('X-Twilio-Signature', '')):
        abort(401)

    params = {
        'CallSid': request.form['CallSid'],
        'From': request.form['From'].replace('\n', '').replace('\r', ''),
        'FromCity': request.form.get('FromCity', ''),
        'FromState': request.form.get('FromState', ''),
        'FromCountry': request.form.get('FromCountry', ''),
        'To': request.form['To'].replace('\n', '').replace('\r', ''),
        'ToCity': request.form.get('ToCity', ''),
        'ToState': request.form.get('ToState', ''),
        'ToCountry': request.form.get('ToCountry', ''),
        'RecordingUrl': request.form['RecordingUrl'],
    }

    msg = MIMEText(render_template('voicemail_email.txt', **params))
    msg['Date'] = email.utils.formatdate()
    msg['From'] = app.config['MAIL_FROM']
    msg['To'] = ", ".join(app.config['MAIL_TO'])
    msg['Message-Id'] = email.utils.make_msgid()
    msg['X-Mailer'] = "mmvoicemail"
    msg['X-Originating-IP'] = '[{}]'.format(request.remote_addr)
    msg['Subject'] = "Voicemail from {}".format(params['From'])

    try:
        send_email(msg)
    except Exception as e:
        app.logger.error(e)

    return send_from_directory(app.static_folder, 'goodbye.xml')


@app.route('/sms', methods=['POST'])
def incoming_sms():
    if not validate_request(request.url, request.form,
                            request.headers.get('X-Twilio-Signature', '')):
        abort(401)

    params = {
        'From': request.form['From'].replace('\n', '').replace('\r', ''),
        'To': request.form['To'].replace('\n', '').replace('\r', ''),
        'Body': request.form['Body'],
    }

    msg = MIMEText(render_template('sms_email.txt', **params))
    msg['Date'] = email.utils.formatdate()
    msg['From'] = app.config['MAIL_FROM']
    msg['To'] = ", ".join(app.config['MAIL_TO'])
    msg['Message-Id'] = email.utils.make_msgid()
    msg['X-Mailer'] = "mmvoicemail"
    msg['X-Originating-IP'] = '[{}]'.format(request.remote_addr)
    msg['Subject'] = "SMS from {}".format(params['From'])

    send_email(msg)

    return ""
