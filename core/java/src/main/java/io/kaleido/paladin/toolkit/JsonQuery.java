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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonUnwrapped;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class JsonQuery {

    public record Query(
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_DEFAULT)
        int limit,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<String> sort,
        @JsonUnwrapped
        Statements statements
    ) {}

    public record Statements(
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<Statements> or,
        @JsonUnwrapped
        Ops ops
    ) {}

    public record Op(
        @JsonProperty
        String field,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_DEFAULT)
        boolean not,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_DEFAULT)
        boolean caseInsensitive
    ) {}

    public record OpSingleValue(
        @JsonUnwrapped
        Op op,
        @JsonProperty
        Object value
    ) {}

    public record OpMultiValue(
        @JsonUnwrapped
        Op op,
        @JsonProperty
        List<Object> values
    ) {}


    public record Ops(
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> equal ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> eq ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> neq ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> like ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> lessThan ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> lt ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> lessThanOrEqual ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> lte ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> greaterThan ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> gt ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> greaterThanOrEqual ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpSingleValue> gte ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpMultiValue> in ,
        @JsonProperty
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<OpMultiValue> nin ,
        @JsonProperty("null")
        @JsonInclude(JsonInclude.Include.NON_EMPTY)
        List<Op> isNull
    ) {}

    public static Builder newBuilder() {
        return new Builder();
    }

    public static class Builder extends StatementsBuilder  {

        @JsonUnwrapped
        private Query query;

        private Builder() {
            super();
            query = new Query(0, new ArrayList<>(), statements);
        }

        public Builder limit(int limit) {
            query = new Query(limit, query.sort(), statements);
            return this;
        }

        public Builder sort(String field) {
            query.sort.add(field);
            return this;
        }

        @Override
        Builder root() {
            return this;
        }

    }

    public static enum Modifier {
        NOT,
        CASE_INSENSITIVE
    }

    private static Op newOp(String field, Modifier ...modifiers) {
        var not = false;
        var caseInsensitive = false;
        for (var m : modifiers) {
            switch (m) {
                case NOT -> not = true;
                case CASE_INSENSITIVE -> caseInsensitive = true;
            }
        }
        return new Op(field, not, caseInsensitive);
    }

    private static class SubStatementsBuilder extends StatementsBuilder {
        private final Builder root;
        private SubStatementsBuilder(Builder root) {
            this.root = root;
        }
        @Override
        Builder root() {
            return root;
        }
    }

    public interface NestedBuilder {
        public StatementsBuilder nested(StatementsBuilder s);
    }

    public abstract static class StatementsBuilder {

        protected final Statements statements = new Statements(
                 new ArrayList<>(),
                new Ops(
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>(),
                         new ArrayList<>()
                )
        );

        abstract Builder root();

        public StatementsBuilder or(NestedBuilder builder) {
            this.statements.or.add(builder.nested(new SubStatementsBuilder(root())).statements);
            return this;
        }

        public StatementsBuilder isEqual(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.eq.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isNotEqual(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.neq.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isLike(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.like.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isLessThan(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.lt.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isLessThanEqual(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.lte.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isGreaterThan(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.gt.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isGreaterThanEqual(String field, Object value, Modifier ...modifiers) {
            this.statements.ops.gte.add(new OpSingleValue(newOp(field, modifiers), value));
            return this;
        }

        public StatementsBuilder isIn(String field, List<Object> values, Modifier ...modifiers) {
            this.statements.ops.in.add(new OpMultiValue(newOp(field, modifiers), values));
            return this;
        }

        public StatementsBuilder isNotIn(String field, List<Object> values, Modifier ...modifiers) {
            this.statements.ops.nin.add(new OpMultiValue(newOp(field, modifiers), values));
            return this;
        }

        public StatementsBuilder isNull(String field, Modifier ...modifiers) {
            this.statements.ops.isNull.add(newOp(field, modifiers));
            return this;
        }

        public final Query build() {
            return root().build();

        }
        public final String json() throws IOException {
            return root().json();
        }

    }
}
