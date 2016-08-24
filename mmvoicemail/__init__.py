from email.mime.text import MIMEText
from flask import Flask, render_template, request, send_from_directory
import email.utils
import smtplib
from . import config

app = Flask(__name__)
app.config.from_object(config)


@app.route('/record/start.xml')
def record_start():
    return send_from_directory(app.static_folder, 'record.xml')


@app.route('/record/finished.xml', methods=['POST'])
def record_finished():
    to_number = request.form['To'].replace('\n', '').replace('\r', '')
    from_number = request.form['From'].replace('\n', '').replace('\r', '')

    msg = MIMEText(render_template(
        'voicemail_email.txt',
        CallSid=request.form['CallSid'],
        From=from_number,
        To=to_number,
        RecordingUrl=request.form['RecordingUrl']))
    msg['Date'] = email.utils.formatdate()
    msg['From'] = app.config['MAIL_FROM']
    msg['To'] = ", ".join(app.config['MAIL_TO'])
    msg['Message-Id'] = email.utils.make_msgid()
    msg['X-Mailer'] = "mmvoicemail"
    msg['Subject'] = "Voicemail from {}".format(from_number)

    s = smtplib.SMTP(app.config['SMTP_SERVER'])
    s.send_message(msg)
    s.quit()

    return send_from_directory(app.static_folder, 'goodbye.xml')


if __name__ == '__main__':
    app.run()
