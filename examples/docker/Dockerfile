FROM python:3.11-slim

WORKDIR /app

# Copy the application
COPY app.py .

# Make the script executable
RUN chmod +x app.py

# Run the application
CMD ["python3", "-u", "app.py"]
