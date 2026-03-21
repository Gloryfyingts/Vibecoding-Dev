FROM flink:1.20.1-scala_2.12-java11

RUN mkdir -p /opt/flink/plugins/s3-fs-hadoop && \
    cp /opt/flink/opt/flink-s3-fs-hadoop-*.jar /opt/flink/plugins/s3-fs-hadoop/

RUN wget -q -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-sql-connector-kafka/3.3.0-1.20/flink-sql-connector-kafka-3.3.0-1.20.jar

RUN echo '\n\
logger.s3a.name = org.apache.hadoop.fs.s3a\n\
logger.s3a.level = DEBUG\n\
logger.awsclient.name = com.amazonaws\n\
logger.awsclient.level = DEBUG\n\
' >> /opt/flink/conf/log4j-console.properties
