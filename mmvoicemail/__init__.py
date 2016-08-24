from email.mime.text import MIMEText
from flask import Flask, render_template, request, send_from_directory
import email.utils
import smtplib
import config

app = Flask(__name__)
app.config.from_object(config)


@app.route('/record/start.xml')
def record_start():
    return send_from_directory(app.static_folder, 'record.xml')


@app.route('/record/finished.xml', methods=['POST'])
def record_finished():
    msg = MIMEText(render_template(
        'voicemail_email.txt',
        CallSid=request.form['CallSid'],
        From=request.form['From'],
        To=request.form['To'],
        RecordingUrl=request.form['RecordingUrl']))
    msg['Date'] = email.utils.formatdate()
    msg['From'] = app.config['MAIL_FROM']
    msg['To'] = ", ".join(app.config['MAIL_TO'])
    msg['Message-Id'] = email.utils.make_msgid()
    msg['X-Mailer'] = "mmvoicemail"
    msg['Subject'] = "Voicemail from {}".format(request.form['From'])

    s = smtplib.SMTP(app.config['SMTP_SERVER'])
    s.sendmail(msg['From'], app.config['MAIL_TO'], msg.as_string())
    s.quit()

    return send_from_directory(app.static_folder, 'goodbye.xml')


if __name__ == '__main__':
    app.run()
