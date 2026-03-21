---------------------------------------------------------------------------
Py4JJavaError                             Traceback (most recent call last)
Cell In[4], line 37
      9 spark = (
     10     SparkSession.builder
     11     .master('local[*]')
   (...)
     32     .getOrCreate()
     33 )
     35 spark.sql('CREATE NAMESPACE IF NOT EXISTS iceberg.db')
---> 37 df = spark.read.json('s3a://raw/events/')
     38 df.printSchema()
     39 print(f'Row count from S3: {df.count()}')

File /usr/local/spark/python/pyspark/sql/readwriter.py:425, in DataFrameReader.json(self, path, schema, primitivesAsString, prefersDecimal, allowComments, allowUnquotedFieldNames, allowSingleQuotes, allowNumericLeadingZero, allowBackslashEscapingAnyCharacter, mode, columnNameOfCorruptRecord, dateFormat, timestampFormat, multiLine, allowUnquotedControlChars, lineSep, samplingRatio, dropFieldIfAllNull, encoding, locale, pathGlobFilter, recursiveFileLookup, modifiedBefore, modifiedAfter, allowNonNumericNumbers)
    423 if type(path) == list:
    424     assert self._spark._sc._jvm is not None
--> 425     return self._df(self._jreader.json(self._spark._sc._jvm.PythonUtils.toSeq(path)))
    426 elif isinstance(path, RDD):
    428     def func(iterator: Iterable) -> Iterable:

File /usr/local/spark/python/lib/py4j-0.10.9.7-src.zip/py4j/java_gateway.py:1322, in JavaMember.__call__(self, *args)
   1316 command = proto.CALL_COMMAND_NAME +\
   1317     self.command_header +\
   1318     args_command +\
   1319     proto.END_COMMAND_PART
   1321 answer = self.gateway_client.send_command(command)
-> 1322 return_value = get_return_value(
   1323     answer, self.gateway_client, self.target_id, self.name)
   1325 for temp_arg in temp_args:
   1326     if hasattr(temp_arg, "_detach"):

File /usr/local/spark/python/pyspark/errors/exceptions/captured.py:179, in capture_sql_exception.<locals>.deco(*a, **kw)
    177 def deco(*a: Any, **kw: Any) -> Any:
    178     try:
--> 179         return f(*a, **kw)
    180     except Py4JJavaError as e:
    181         converted = convert_exception(e.java_exception)

File /usr/local/spark/python/lib/py4j-0.10.9.7-src.zip/py4j/protocol.py:326, in get_return_value(answer, gateway_client, target_id, name)
    324 value = OUTPUT_CONVERTER[type](answer[2:], gateway_client)
    325 if answer[1] == REFERENCE_TYPE:
--> 326     raise Py4JJavaError(
    327         "An error occurred while calling {0}{1}{2}.\n".
    328         format(target_id, ".", name), value)
    329 else:
    330     raise Py4JError(
    331         "An error occurred while calling {0}{1}{2}. Trace:\n{3}\n".
    332         format(target_id, ".", name, value))

Py4JJavaError: An error occurred while calling o64.json.
: java.lang.RuntimeException: java.lang.ClassNotFoundException: Class org.apache.hadoop.fs.s3a.S3AFileSystem not found
	at org.apache.hadoop.conf.Configuration.getClass(Configuration.java:2688)
	at org.apache.hadoop.fs.FileSystem.getFileSystemClass(FileSystem.java:3431)
	at org.apache.hadoop.fs.FileSystem.createFileSystem(FileSystem.java:3466)
	at org.apache.hadoop.fs.FileSystem.access$300(FileSystem.java:174)
	at org.apache.hadoop.fs.FileSystem$Cache.getInternal(FileSystem.java:3574)
	at org.apache.hadoop.fs.FileSystem$Cache.get(FileSystem.java:3521)
	at org.apache.hadoop.fs.FileSystem.get(FileSystem.java:540)
	at org.apache.hadoop.fs.Path.getFileSystem(Path.java:365)
	at org.apache.spark.sql.execution.datasources.DataSource$.$anonfun$checkAndGlobPathIfNecessary$1(DataSource.scala:724)
	at scala.collection.immutable.List.map(List.scala:293)
	at org.apache.spark.sql.execution.datasources.DataSource$.checkAndGlobPathIfNecessary(DataSource.scala:722)
	at org.apache.spark.sql.execution.datasources.DataSource.checkAndGlobPathIfNecessary(DataSource.scala:551)
	at org.apache.spark.sql.execution.datasources.DataSource.resolveRelation(DataSource.scala:404)
	at org.apache.spark.sql.DataFrameReader.loadV1Source(DataFrameReader.scala:229)
	at org.apache.spark.sql.DataFrameReader.$anonfun$load$2(DataFrameReader.scala:211)
	at scala.Option.getOrElse(Option.scala:189)
	at org.apache.spark.sql.DataFrameReader.load(DataFrameReader.scala:211)
	at org.apache.spark.sql.DataFrameReader.json(DataFrameReader.scala:362)
	at java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke0(Native Method)
	at java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke(NativeMethodAccessorImpl.java:77)
	at java.base/jdk.internal.reflect.DelegatingMethodAccessorImpl.invoke(DelegatingMethodAccessorImpl.java:43)
	at java.base/java.lang.reflect.Method.invoke(Method.java:569)
	at py4j.reflection.MethodInvoker.invoke(MethodInvoker.java:244)
	at py4j.reflection.ReflectionEngine.invoke(ReflectionEngine.java:374)
	at py4j.Gateway.invoke(Gateway.java:282)
	at py4j.commands.AbstractCommand.invokeMethod(AbstractCommand.java:132)
	at py4j.commands.CallCommand.execute(CallCommand.java:79)
	at py4j.ClientServerConnection.waitForCommands(ClientServerConnection.java:182)
	at py4j.ClientServerConnection.run(ClientServerConnection.java:106)
	at java.base/java.lang.Thread.run(Thread.java:840)
Caused by: java.lang.ClassNotFoundException: Class org.apache.hadoop.fs.s3a.S3AFileSystem not found
	at org.apache.hadoop.conf.Configuration.getClassByName(Configuration.java:2592)
	at org.apache.hadoop.conf.Configuration.getClass(Configuration.java:2686)
	... 29 more