# locustfile2.py
# A Locust script to test the Noroi HTTP backend using HttpUser
# Refactored to mirror k6 scenarios using separate User classes

from locust import HttpUser, task, constant

class FixedDelayUser(HttpUser):
    wait_time = constant(0)
    weight = 1
    
    @task
    def test(self):
        self.client.get("/respond?delay=300ms&size=10kb", name="Fixed Delay")

class RandomDelayUser(HttpUser):
    wait_time = constant(0)
    weight = 1
    
    @task
    def test(self):
        self.client.get("/respond?delay_min=100ms&delay_max=500ms&error_rate=0.05", name="Random Delay")

class HighErrorRateUser(HttpUser):
    wait_time = constant(0)
    weight = 1
    
    @task
    def test(self):
        self.client.get("/respond?error_rate=0.2", name="High Error Rate")

class CpuLoadUser(HttpUser):
    wait_time = constant(0)
    weight = 1
    
    @task
    def test(self):
        self.client.get("/respond?cpu_ms=100", name="CPU Load")
