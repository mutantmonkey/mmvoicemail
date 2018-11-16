from email.mime.text import MIMEText
from flask import abort, Flask, render_template, request, send_from_directory
from functools import wraps
import email.utils
import os
import smtplib
from twilio.request_validator import RequestValidator

app = Flask(__name__)
app.config.update({
    'PROXY_FIX': False,
    'PROXY_FIX_NUM_PROXIES': 1,
})
app.config.from_json(os.environ.get('APP_CONFIG_PATH', 'config.json'))

if app.config['PROXY_FIX']:
    from werkzeug.contrib.fixers import ProxyFix
    app.wsgi_app = ProxyFix(app.wsgi_app,
                            num_proxies=app.config['PROXY_FIX_NUM_PROXIES'])


def validate_twilio_request(f):
    """Validates that incoming requests genuinely originated from Twilio"""
    @wraps(f)
    def decorated_function(*args, **kwargs):
        # Create an instance of the RequestValidator class
        validator = RequestValidator(app.config['TWILIO_AUTH_TOKEN'])

        # Validate the request using its URL, POST data,
        # and X-TWILIO-SIGNATURE header
        request_valid = validator.validate(
            request.url,
            request.form,
            request.headers.get('X-TWILIO-SIGNATURE', ''))

        # Continue processing the request if it's valid, return a 403 error if
        # it's not
        if request_valid:
            return f(*args, **kwargs)
        else:
            return abort(403)
    return decorated_function


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


@validate_twilio_request
@app.route('/record/start.xml', methods=['POST'])
def record_start():
    return send_from_directory(app.static_folder, 'record.xml')


@validate_twilio_request
@app.route('/record/finished.xml', methods=['POST'])
def record_finished():
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


@validate_twilio_request
@app.route('/sms', methods=['POST'])
def incoming_sms():
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
