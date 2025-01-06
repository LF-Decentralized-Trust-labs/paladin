/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package io.kaleido.paladin.toolkit;

import io.grpc.ChannelLogger;
import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.kaleido.paladin.logging.PaladinLogging;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioDomainSocketChannel;
import io.netty.handler.logging.LoggingHandler;
import io.netty.util.internal.logging.InternalLogLevel;
import io.netty.util.internal.logging.InternalLogger;
import io.netty.util.internal.logging.InternalLoggerFactory;
import io.netty.util.internal.logging.Log4J2LoggerFactory;
import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.spi.ExtendedLoggerWrapper;

import java.net.UnixDomainSocketAddress;
import java.util.concurrent.*;

public class GRPCTargetConnector {

    static {
        InternalLoggerFactory.setDefaultFactory(new InternalLoggerFactory() {
            @Override
            protected InternalLogger newInstance(String s) {

                final var logger = PaladinLogging.getLogger(s);

                return new InternalLogger() {
                    @Override
                    public String name() {
                        return s;
                    }

                    @Override
                    public boolean isTraceEnabled() {
                        return logger.isTraceEnabled();
                    }

                    @Override
                    public void trace(String s) {
                        logger.trace( s);
                    }

                    @Override
                    public void trace(String s, Object o) {
                        logger.trace(s, o);
                    }

                    @Override
                    public void trace(String s, Object o, Object o1) {
                        logger.trace(s, o, o1);
                    }

                    @Override
                    public void trace(String s, Object... objects) {
                        logger.trace(s, objects);
                    }

                    @Override
                    public void trace(String s, Throwable throwable) {
                        logger.trace(s, throwable);
                    }

                    @Override
                    public void trace(Throwable throwable) {
                        logger.trace(throwable);
                    }

                    @Override
                    public boolean isDebugEnabled() {
                        return logger.isDebugEnabled();
                    }

                    @Override
                    public void debug(String s) {
                        logger.debug( s);
                    }

                    @Override
                    public void debug(String s, Object o) {
                        logger.debug(s, o);
                    }

                    @Override
                    public void debug(String s, Object o, Object o1) {
                        logger.debug(s, o, o1);
                    }

                    @Override
                    public void debug(String s, Object... objects) {
                        logger.debug(s, objects);
                    }

                    @Override
                    public void debug(String s, Throwable throwable) {
                        logger.debug(s, throwable);
                    }

                    @Override
                    public void debug(Throwable throwable) {
                        logger.debug(throwable);
                    }

                    @Override
                    public boolean isInfoEnabled() {
                        return logger.isInfoEnabled();
                    }

                    @Override
                    public void info(String s) {
                        logger.info(s);
                    }

                    @Override
                    public void info(String s, Object o) {
                        logger.info(s, o);
                    }

                    @Override
                    public void info(String s, Object o, Object o1) {
                        logger.info(s, o, o1);
                    }

                    @Override
                    public void info(String s, Object... objects) {
                        logger.info(s, objects);
                    }

                    @Override
                    public void info(String s, Throwable throwable) {
                        logger.info(s, throwable);
                    }

                    @Override
                    public void info(Throwable throwable) {
                        logger.info(throwable);
                    }

                    @Override
                    public boolean isWarnEnabled() {
                        return logger.isWarnEnabled();
                    }

                    @Override
                    public void warn(String s) {
                        logger.warn(s);
                    }

                    @Override
                    public void warn(String s, Object o) {
                        logger.warn(s, o);
                    }

                    @Override
                    public void warn(String s, Object... objects) {
                        logger.warn(s, objects);
                    }

                    @Override
                    public void warn(String s, Object o, Object o1) {
                        logger.warn(s, o, o1);
                    }

                    @Override
                    public void warn(String s, Throwable throwable) {
                        logger.warn(s, throwable);
                    }

                    @Override
                    public void warn(Throwable throwable) {
                        logger.warn(throwable);
                    }

                    @Override
                    public boolean isErrorEnabled() {
                        return logger.isErrorEnabled();
                    }

                    @Override
                    public void error(String s) {
                        logger.error(s);
                    }

                    @Override
                    public void error(String s, Object o) {
                        logger.error(s, o);
                    }

                    @Override
                    public void error(String s, Object o, Object o1) {
                        logger.error(s, o, o1);
                    }

                    @Override
                    public void error(String s, Object... objects) {
                        logger.error(s, objects);
                    }

                    @Override
                    public void error(String s, Throwable throwable) {
                        logger.error(s, throwable);
                    }

                    @Override
                    public void error(Throwable throwable) {
                        logger.error(throwable);
                    }

                    private Level mapLevel(InternalLogLevel internalLogLevel) {
                        return switch (internalLogLevel) {
                            case DEBUG -> Level.DEBUG;
                            case ERROR -> Level.ERROR;
                            case INFO -> Level.INFO;
                            case WARN -> Level.WARN;
                            case TRACE -> Level.TRACE;
                        };
                    }

                    @Override
                    public boolean isEnabled(InternalLogLevel internalLogLevel) {
                        return logger.isEnabled(mapLevel(internalLogLevel));
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, String s) {
                        logger.log(mapLevel(internalLogLevel), s);
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, String s, Object o) {
                        logger.log(mapLevel(internalLogLevel), s, o);
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, String s, Object o, Object o1) {
                        logger.log(mapLevel(internalLogLevel), s, o, o1);
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, String s, Object... objects) {
                        logger.log(mapLevel(internalLogLevel), s, objects);
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, String s, Throwable throwable) {
                        logger.log(mapLevel(internalLogLevel), s, throwable);
                    }

                    @Override
                    public void log(InternalLogLevel internalLogLevel, Throwable throwable) {
                        logger.log(mapLevel(internalLogLevel), throwable);
                    }
                };
            }
        });
    }

    public static ManagedChannel connect(String grpcTarget) {
        NettyChannelBuilder channelBuilder;
        if (grpcTarget.startsWith("unix:")) {
            String socketFile = grpcTarget.replaceFirst("unix:", "");
            channelBuilder = NettyChannelBuilder.forAddress(UnixDomainSocketAddress.of(socketFile));
        } else {
            channelBuilder = NettyChannelBuilder.forTarget(grpcTarget);
        }
        return channelBuilder
                .eventLoopGroup(new NioEventLoopGroup())
                .channelType(NioDomainSocketChannel.class)
                .usePlaintext()
                .build();
    }
}
