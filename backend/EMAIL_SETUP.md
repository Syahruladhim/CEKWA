# Email Configuration Setup

## Overview
This application uses Gmail SMTP to send OTP verification emails and password reset emails.

## Development Mode (Default)
If email credentials are not configured, the application will run in development mode:
- OTP codes will be printed to the console instead of being sent via email
- You can see the OTP in the backend console logs
- No email configuration is required for development

## Production Email Setup

### 1. Gmail App Password Setup
1. Go to your Google Account settings
2. Enable 2-Factor Authentication if not already enabled
3. Go to Security → App passwords
4. Generate a new app password for "Mail"
5. Copy the 16-character app password

### 2. Environment Configuration
Create or update your environment file with the following variables:

```env
# Email SMTP Configuration
EMAIL_USERNAME=your-email@gmail.com
EMAIL_PASSWORD=your-16-char-app-password
EMAIL_FROM=your-email@gmail.com
EMAIL_FROM_NAME=WhatsApp Defender
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
```

### 3. Example Configuration
```env
EMAIL_USERNAME=myapp@gmail.com
EMAIL_PASSWORD=abcd efgh ijkl mnop
EMAIL_FROM=myapp@gmail.com
EMAIL_FROM_NAME=WhatsApp Defender
```

## Testing Email Configuration

### Development Mode
When email credentials are not configured, you'll see output like this in the console:
```
Email credentials not configured, using development mode
=== OTP EMAIL (DEV MODE) ===
To: user@example.com
OTP Code: 123456
Expires in: 10 minutes
==========================
```

### Production Mode
When email credentials are configured, you'll see:
```
Attempting to send email via smtp.gmail.com:587 from myapp@gmail.com to user@example.com
OTP email sent successfully to user@example.com
```

## Troubleshooting

### Common Issues
1. **"email credentials not configured"** - Set EMAIL_USERNAME and EMAIL_PASSWORD
2. **"authentication failed"** - Use App Password, not your regular Gmail password
3. **"connection refused"** - Check firewall settings and port 587
4. **"less secure app access"** - Use App Password instead of enabling less secure apps

### Gmail Security Settings
- Do NOT enable "Less secure app access"
- Use App Passwords instead
- Make sure 2-Factor Authentication is enabled

## Security Notes
- Never commit email passwords to version control
- Use environment variables for sensitive data
- App passwords are more secure than regular passwords
- Consider using dedicated email service for production (SendGrid, Mailgun, etc.) 