import os
from typing import Optional


class UserService:
    def __init__(self, name: str):
        self.name = name

    def get_name(self) -> str:
        return self.name


def create_service(name: str) -> "UserService":
    return UserService(name)
