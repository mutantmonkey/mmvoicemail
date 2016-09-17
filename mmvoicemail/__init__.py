from email.mime.text import MIMEText
from flask import Flask, render_template, request, send_from_directory
import email.utils
import os
import smtplib

app = Flask(__name__)
app.config.from_pyfile(os.environ.get('APP_CONFIG_PATH', 'config.py'))


@app.route('/record/start.xml', methods=['GET', 'POST'])
def record_start():
    return send_from_directory(app.static_folder, 'record.xml')


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
        s = smtplib.SMTP(app.config['SMTP_SERVER'])
        s.send_message(msg)
        s.quit()
    except Exception as e:
        app.logger.error(e)

    return send_from_directory(app.static_folder, 'goodbye.xml')
