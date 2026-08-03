from setuptools import find_packages, setup

setup(
    name="vigil-sdk",
    version="0.1.0",
    description="Lightweight event tracking client for vigil",
    packages=find_packages(exclude=["examples", "examples.*"]),
    python_requires=">=3.7",
    install_requires=[],
)
