# D:\Myprojects\aqi_p\aqi_django_project\dashboard\views.py

from django.shortcuts import render
from django.http import JsonResponse
import requests
import json
from datetime import datetime

# Define your FastAPI endpoint URL
# IMPORTANT: Make sure your FastAPI app is running on this address and port!
# FASTAPI_ANOMALY_CHECK_URL = "http://127.0.0.1:8000/predict_anomaly/"
# D:\Myprojects\aqi_p\aqi_django_project\dashboard\views.py

FASTAPI_ANOMALY_CHECK_URL = "http://127.0.0.1:8001/predict_anomaly/"
def get_realtime_aqi_and_check_anomaly(request):
    """
    A Django view that simulates fetching a new AQI reading
    and then calls the FastAPI service to check for anomalies.
    """
    # --- STEP 1: Simulate getting new real-time AQI data ---
    # In a real-world scenario, you would replace this section
    # with code that fetches the *latest* AQI reading
    # For example:
    # - Polling your MongoDB for the newest record
    # - Receiving data from your Go ingestion service
    # - From a form submission on your Django site
    
    # For demonstration, let's use a sample data point with the current time
    # This data mirrors the structure FastAPI expects.
    current_timestamp = datetime.now()
    latest_aqi_value = 155.0  # Example: A high reading to test anomaly detection
    latest_temperature = 35.0
    latest_humidity = 70.0
    latest_wind_speed = 6.5

    # --- STEP 2: Prepare data for FastAPI and make the POST request ---
    payload = {
        "timestamp": current_timestamp.isoformat() + "Z", # ISO 8601 format with 'Z' for UTC
        "aqi_value": latest_aqi_value,
        "temperature": latest_temperature,
        "humidity": latest_humidity,
        "wind_speed": latest_wind_speed
    }

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json" # Request JSON response
    }

    anomaly_data = None # Initialize to None; will hold FastAPI's response

    try:
        # Make the POST request to your FastAPI service
        response = requests.post(FASTAPI_ANOMALY_CHECK_URL, headers=headers, data=json.dumps(payload))
        response.raise_for_status() # Raise an HTTPError for bad responses (4xx or 5xx)

        # Parse the JSON response from FastAPI
        anomaly_data = response.json()

        # Print to your Django terminal for debugging (not shown in browser)
        print(f"\n--- FastAPI Anomaly Check Result for {current_timestamp} ---")
        print(f"  Actual AQI: {latest_aqi_value}")
        print(f"  Is Anomaly: {anomaly_data.get('is_anomaly')}")
        print(f"  Predicted Range: [{anomaly_data.get('aqi_lower_bound'):.2f} - {anomaly_data.get('aqi_upper_bound'):.2f}]")
        print(f"  Deviation: {anomaly_data.get('deviation'):.2f}")
        print("--------------------------------------------------\n")

    except requests.exceptions.ConnectionError:
        print("ERROR: Could not connect to FastAPI anomaly detection service. Is it running?")
        anomaly_data = {"error": "FastAPI service not reachable. Ensure it's running."}
    except requests.exceptions.HTTPError as e:
        print(f"ERROR: HTTP Error from FastAPI: {e}")
        print(f"FastAPI Response Content: {e.response.text}") # Print FastAPI's error message
        anomaly_data = {"error": f"FastAPI HTTP error: {e}. Check FastAPI logs."}
    except json.JSONDecodeError:
        print("ERROR: Could not decode JSON response from FastAPI. Unexpected response format.")
        anomaly_data = {"error": "Invalid JSON response from FastAPI."}
    except Exception as e:
        print(f"ERROR: An unexpected error occurred during FastAPI call: {e}")
        anomaly_data = {"error": f"An unexpected error: {e}"}

    # --- STEP 3: Pass data to your Django template ---
    context = {
        'current_aqi_data': {
            'timestamp': current_timestamp,
            'aqi_value': latest_aqi_value,
            'temperature': latest_temperature,
            'humidity': latest_humidity,
            'wind_speed': latest_wind_speed,
            'anomaly_check': anomaly_data # This contains the full FastAPI response
        }
    }

    # Render the HTML template
    return render(request, 'dashboard/aqi_status.html', context)