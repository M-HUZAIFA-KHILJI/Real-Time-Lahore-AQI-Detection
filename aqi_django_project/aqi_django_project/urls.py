from django.contrib import admin
from django.urls import path, include # <-- Make sure 'include' is imported here!

urlpatterns = [
    path('admin/', admin.site.urls),
    path('dashboard/', include('dashboard.urls')), # <-- Add this line!
]