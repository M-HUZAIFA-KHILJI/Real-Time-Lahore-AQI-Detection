# D:\Myprojects\aqi_p\aqi_django_project\dashboard\urls.py

from django.urls import path
from . import views # Import views from the current app

urlpatterns = [
    # This path means that if someone goes to /dashboard/aqi-status/
    # it will call the function named 'get_realtime_aqi_and_check_anomaly'
    # from your views.py file.
    path('aqi-status/', views.get_realtime_aqi_and_check_anomaly, name='aqi_status'),
    # You can add more paths here as you build out your dashboard
]