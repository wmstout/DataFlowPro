FROM maven:3.8-eclipse-temurin-11 AS builder

WORKDIR /build

COPY pom.xml ./
COPY src ./src

RUN mvn clean package dependency:copy-dependencies -DskipTests

FROM eclipse-temurin:11-jre

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /build/target/dataflow-pro-java-2.4.1.jar /app/app.jar
COPY --from=builder /build/target/dependency /app/lib

# Runtime directories and seed data for J5 / FP-J4 path traversal
RUN mkdir -p /data/exports /data/reports \
    && echo "id,name,rows\n1,dataset_1,100" > /data/exports/export_sample.csv \
    && echo "Java Analytics Engine Summary" > /data/reports/report_2024.txt

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/datasets/search?q=rev || exit 1

CMD ["java", "-cp", "/app/app.jar:/app/lib/*", "com.dataflow.DataFlowService", "8080"]
