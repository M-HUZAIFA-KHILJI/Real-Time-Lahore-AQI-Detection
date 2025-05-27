# api.py

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import joblib
import pandas as pd
from datetime import datetime, timedelta
import os
import logging

# --- Setup Logging ---
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

app = FastAPI(title="AQI Anomaly Detection API")

# --- Configuration ---
MODEL_PATH = os.getenv("MODEL_PATH", "prophet_aqi_model.pkl") # Path to your saved model

# Global variable to hold the loaded model
prophet_model = None

# --- Model Loading on Startup ---
@app.on_event("startup")
async def load_model():
    """Loads the trained Prophet model when the FastAPI application starts."""
    global prophet_model
    if not os.path.exists(MODEL_PATH):
        logger.error(f"Model file not found at {MODEL_PATH}. Please ensure retrain_model.py has run successfully.")
        raise RuntimeError(f"Model file not found: {MODEL_PATH}")
    try:
        prophet_model = joblib.load(MODEL_PATH)
        logger.info(f"Prophet model loaded successfully from {MODEL_PATH}")
    except Exception as e:
        logger.exception(f"Error loading model from {MODEL_PATH}: {e}")
        raise RuntimeError(f"Failed to load model: {e}")

# --- Input Data Model for Real-time Prediction ---
class AQIInput(BaseModel):
    timestamp: datetime # The timestamp of the new AQI reading
    aqi_value: float    # The actual AQI value for anomaly check
    temperature: float  # Regressor
    humidity: float     # Regressor
    wind_speed: float   # Regressor
    # Add any other regressors your Prophet model uses (e.g., co, no2, so2, o3, etc.)

# --- Output Data Model for Prediction Result ---
class AnomalyPrediction(BaseModel):
    timestamp: datetime
    actual_aqi: float
    predicted_mean_aqi: float # yhat
    aqi_lower_bound: float    # yhat_lower
    aqi_upper_bound: float    # yhat_upper
    is_anomaly: bool          # True if actual_aqi is outside the bounds
    deviation: float          # How far actual_aqi is from the nearest bound (0 if not anomaly)

# --- Prediction Endpoint ---
@app.post("/predict_anomaly/", response_model=AnomalyPrediction)
async def predict_anomaly(data: AQIInput):
    """
    Receives a single new AQI data point and predicts if it's an anomaly.
    """
    if prophet_model is None:
        logger.error("Prediction requested, but model is not loaded.")
        raise HTTPException(status_code=503, detail="Model not loaded. Service unavailable.")

    logger.info(f"Received data for prediction: {data.timestamp}, AQI: {data.aqi_value}")

    # --- FIX APPLIED HERE ---
    # Ensure the timestamp is naive (no timezone) as Prophet often expects this
    # FastAPI's datetime parsing might include timezone info if 'Z' or offset is present
    naive_timestamp = data.timestamp.replace(tzinfo=None)
    
    input_df = pd.DataFrame([{
        'ds': naive_timestamp,  # Use the naive timestamp here
        'temperature': data.temperature,
        'humidity': data.humidity,
        'wind_speed': data.wind_speed
        # Add other regressors here if they are part of your model
    }])

    try:
        # Generate a forecast for this single point
        forecast = prophet_model.predict(input_df)

        # Extract predicted mean and confidence intervals
        predicted_mean_aqi = forecast['yhat'].iloc[0]
        aqi_lower_bound = forecast['yhat_lower'].iloc[0]
        aqi_upper_bound = forecast['yhat_upper'].iloc[0]

        is_anomaly = False
        deviation = 0.0

        # Check if the actual AQI falls outside the predicted confidence interval
        if data.aqi_value < aqi_lower_bound:
            is_anomaly = True
            deviation = data.aqi_value - aqi_lower_bound # Negative if below lower bound
        elif data.aqi_value > aqi_upper_bound:
            is_anomaly = True
            deviation = data.aqi_value - aqi_upper_bound # Positive if above upper bound

        logger.info(f"Prediction complete. Actual: {data.aqi_value}, Predicted: {predicted_mean_aqi:.2f}, Bounds: [{aqi_lower_bound:.2f}, {aqi_upper_bound:.2f}], Anomaly: {is_anomaly}")

        return AnomalyPrediction(
            timestamp=data.timestamp, # Return the original timestamp, even if it had tzinfo
            actual_aqi=data.aqi_value,
            predicted_mean_aqi=predicted_mean_aqi,
            aqi_lower_bound=aqi_lower_bound,
            aqi_upper_bound=aqi_upper_bound,
            is_anomaly=is_anomaly,
            deviation=deviation
        )

    except Exception as e:
        logger.exception(f"Error during prediction for timestamp {data.timestamp}: {e}")
        raise HTTPException(status_code=500, detail=f"Prediction failed: {e}")

# --- Optional: Health Check Endpoint ---
@app.get("/health/")
async def health_check():
    """Returns the status of the API and model loading."""
    if prophet_model is not None:
        return {"status": "ok", "model_loaded": True, "message": "API and model are operational."}
    else:
        raise HTTPException(status_code=503, detail="Model not loaded. Service not fully ready.")