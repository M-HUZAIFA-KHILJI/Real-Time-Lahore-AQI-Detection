# retrain_model.py

import pandas as pd
import numpy as np
from pymongo import MongoClient
from prophet import Prophet
import joblib # For saving/loading the model
import logging
import os
from datetime import datetime

# --- Configuration (Make sure these match your project's config) ---
MONGO_CONNECTION_STRING = "mongodb+srv://MHK_Technologies:nISQuhdTNSo1N7Lq@cluster0.lgzcnm2.mongodb.net/weather_aqi_db?retryWrites=true&w=majority"
MONGO_DB_NAME = os.getenv("MONGO_DB_NAME", "weather_aqi_db") # This line is now correctly separated
MONGO_COLLECTION_NAME = os.getenv("MONGO_COLLECTION_NAME", "city_data")
CITY_FOCUS = os.getenv("CITY_FOCUS", "Lahore") # Ensure this matches your target city
MODEL_SAVE_PATH = os.getenv("MODEL_SAVE_PATH", "prophet_aqi_model.pkl") # Where to save the updated model

# --- Setup Logging ---
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

# --- Data Loading and Preparation Function (Adapted from your Cell 3) ---
def load_and_prepare_aqi_data_for_retraining():
    logger.info(f"Connecting to MongoDB Atlas database '{MONGO_DB_NAME}' collection '{MONGO_COLLECTION_NAME}'...")
    try:
        client = MongoClient(MONGO_CONNECTION_STRING)
        db = client[MONGO_DB_NAME]
        collection = db[MONGO_COLLECTION_NAME]

        logger.info(f"Fetching ALL documents from collection: {MONGO_COLLECTION_NAME} for city: {CITY_FOCUS} (sorted by timestamp)...")
        # Filter by city directly in MongoDB for efficiency
        cursor = collection.find({"city": CITY_FOCUS}).sort("timestamp", 1)
        df_raw = pd.DataFrame(list(cursor))
        client.close() # Close connection immediately after fetching

        if df_raw.empty:
            logger.error(f"No data found for city '{CITY_FOCUS}' in collection '{MONGO_COLLECTION_NAME}'. Cannot retrain.")
            return pd.DataFrame() # Return empty DataFrame

        df_city_filtered = df_raw.copy()

        # Rename columns for Prophet and ensure correct types
        df_city_filtered.rename(columns={'timestamp': 'ds', 'aqi': 'y'}, inplace=True)

        df_city_filtered['ds'] = pd.to_datetime(df_city_filtered['ds'])
        df_city_filtered['y'] = pd.to_numeric(df_city_filtered['y'], errors='coerce')

        regressor_cols = ['temperature', 'humidity', 'wind_speed']
        for col in regressor_cols:
            if col in df_city_filtered.columns:
                df_city_filtered[col] = pd.to_numeric(df_city_filtered[col], errors='coerce')
            else:
                logger.warning(f"MongoDB document does not contain expected regressor '{col}'. Skipping.")

        initial_rows = df_city_filtered.shape[0]
        cols_to_check = ['ds', 'y'] + [col for col in regressor_cols if col in df_city_filtered.columns]
        df_city_filtered.dropna(subset=cols_to_check, inplace=True)

        if df_city_filtered.shape[0] < initial_rows:
            logger.warning(f"Dropped {initial_rows - df_city_filtered.shape[0]} rows due to missing values in critical columns.")

        df_city_filtered.sort_values('ds', inplace=True)

        logger.info(f"Successfully loaded and prepared {df_city_filtered.shape[0]} documents for {CITY_FOCUS}.")
        return df_city_filtered

    except Exception as e:
        logger.exception(f"Error loading data for retraining: {e}")
        return pd.DataFrame()

# --- Model Training Function (Adapted from your Cell 4) ---
def train_and_save_prophet_model(df, model_path):
    if df.empty:
        logger.error("No data provided for training. Skipping model training.")
        return False

    logger.info("Initializing Prophet model with weather regressors...")
    model = Prophet(
        yearly_seasonality=True,
        weekly_seasonality=True,
        daily_seasonality=True
    )

    regressor_cols = ['temperature', 'humidity', 'wind_speed']
    for col in regressor_cols:
        if col in df.columns:
            logger.info(f"Adding regressor: {col}")
            model.add_regressor(col)
        else:
            logger.warning(f"Regressor '{col}' not found in DataFrame. Skipping.")

    logger.info(f"Fitting Prophet model with {df.shape[0]} data points...")
    try:
        model.fit(df)
        logger.info("Prophet model trained successfully.")

        # Save the trained model
        joblib.dump(model, model_path)
        logger.info(f"Prophet model saved to {model_path}")
        return True
    except Exception as e:
        logger.exception(f"Error during model training or saving: {e}")
        return False

# --- Main execution block for the retraining script ---
if __name__ == "__main__":
    logger.info("Starting automated model retraining process...")
    
    # Load and prepare data
    df_prepared = load_and_prepare_aqi_data_for_retraining()

    if not df_prepared.empty:
        # Train and save the model
        if train_and_save_prophet_model(df_prepared, MODEL_SAVE_PATH):
            logger.info("Automated model retraining completed successfully.")
        else:
            logger.error("Automated model retraining failed.")
    else:
        logger.error("No data available to retrain the model.")