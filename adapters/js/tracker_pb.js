/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
"use strict";

var $protobuf = require("protobufjs/minimal");

// Common aliases
var $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
var $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

$root.pb = (function() {

    /**
     * Namespace pb.
     * @exports pb
     * @namespace
     */
    var pb = {};

    pb.RegisterRequest = (function() {

        /**
         * Properties of a RegisterRequest.
         * @memberof pb
         * @interface IRegisterRequest
         * @property {number|null} [pid] RegisterRequest pid
         */

        /**
         * Constructs a new RegisterRequest.
         * @memberof pb
         * @classdesc Represents a RegisterRequest.
         * @implements IRegisterRequest
         * @constructor
         * @param {pb.IRegisterRequest=} [properties] Properties to set
         */
        function RegisterRequest(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * RegisterRequest pid.
         * @member {number} pid
         * @memberof pb.RegisterRequest
         * @instance
         */
        RegisterRequest.prototype.pid = 0;

        /**
         * Creates a new RegisterRequest instance using the specified properties.
         * @function create
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest=} [properties] Properties to set
         * @returns {pb.RegisterRequest} RegisterRequest instance
         */
        RegisterRequest.create = function create(properties) {
            return new RegisterRequest(properties);
        };

        /**
         * Encodes the specified RegisterRequest message. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest} message RegisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            return writer;
        };

        /**
         * Encodes the specified RegisterRequest message, length delimited. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest} message RegisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.RegisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.RegisterRequest} RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.RegisterRequest();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.RegisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.RegisterRequest} RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a RegisterRequest message.
         * @function verify
         * @memberof pb.RegisterRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        RegisterRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            return null;
        };

        /**
         * Creates a RegisterRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.RegisterRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.RegisterRequest} RegisterRequest
         */
        RegisterRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.RegisterRequest)
                return object;
            var message = new $root.pb.RegisterRequest();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a RegisterRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.RegisterRequest} message RegisterRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        RegisterRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults)
                object.pid = 0;
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            return object;
        };

        /**
         * Converts this RegisterRequest to JSON.
         * @function toJSON
         * @memberof pb.RegisterRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        RegisterRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for RegisterRequest
         * @function getTypeUrl
         * @memberof pb.RegisterRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        RegisterRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.RegisterRequest";
        };

        return RegisterRequest;
    })();

    pb.RegisterResponse = (function() {

        /**
         * Properties of a RegisterResponse.
         * @memberof pb
         * @interface IRegisterResponse
         * @property {boolean|null} [success] RegisterResponse success
         * @property {string|null} [message] RegisterResponse message
         */

        /**
         * Constructs a new RegisterResponse.
         * @memberof pb
         * @classdesc Represents a RegisterResponse.
         * @implements IRegisterResponse
         * @constructor
         * @param {pb.IRegisterResponse=} [properties] Properties to set
         */
        function RegisterResponse(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * RegisterResponse success.
         * @member {boolean} success
         * @memberof pb.RegisterResponse
         * @instance
         */
        RegisterResponse.prototype.success = false;

        /**
         * RegisterResponse message.
         * @member {string} message
         * @memberof pb.RegisterResponse
         * @instance
         */
        RegisterResponse.prototype.message = "";

        /**
         * Creates a new RegisterResponse instance using the specified properties.
         * @function create
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse=} [properties] Properties to set
         * @returns {pb.RegisterResponse} RegisterResponse instance
         */
        RegisterResponse.create = function create(properties) {
            return new RegisterResponse(properties);
        };

        /**
         * Encodes the specified RegisterResponse message. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse} message RegisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified RegisterResponse message, length delimited. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse} message RegisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.RegisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.RegisterResponse} RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.RegisterResponse();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.RegisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.RegisterResponse} RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a RegisterResponse message.
         * @function verify
         * @memberof pb.RegisterResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        RegisterResponse.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates a RegisterResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.RegisterResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.RegisterResponse} RegisterResponse
         */
        RegisterResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.RegisterResponse)
                return object;
            var message = new $root.pb.RegisterResponse();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from a RegisterResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.RegisterResponse} message RegisterResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        RegisterResponse.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.success = false;
                object.message = "";
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this RegisterResponse to JSON.
         * @function toJSON
         * @memberof pb.RegisterResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        RegisterResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for RegisterResponse
         * @function getTypeUrl
         * @memberof pb.RegisterResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        RegisterResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.RegisterResponse";
        };

        return RegisterResponse;
    })();

    pb.UnregisterRequest = (function() {

        /**
         * Properties of an UnregisterRequest.
         * @memberof pb
         * @interface IUnregisterRequest
         * @property {number|null} [pid] UnregisterRequest pid
         */

        /**
         * Constructs a new UnregisterRequest.
         * @memberof pb
         * @classdesc Represents an UnregisterRequest.
         * @implements IUnregisterRequest
         * @constructor
         * @param {pb.IUnregisterRequest=} [properties] Properties to set
         */
        function UnregisterRequest(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * UnregisterRequest pid.
         * @member {number} pid
         * @memberof pb.UnregisterRequest
         * @instance
         */
        UnregisterRequest.prototype.pid = 0;

        /**
         * Creates a new UnregisterRequest instance using the specified properties.
         * @function create
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest=} [properties] Properties to set
         * @returns {pb.UnregisterRequest} UnregisterRequest instance
         */
        UnregisterRequest.create = function create(properties) {
            return new UnregisterRequest(properties);
        };

        /**
         * Encodes the specified UnregisterRequest message. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest} message UnregisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            return writer;
        };

        /**
         * Encodes the specified UnregisterRequest message, length delimited. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest} message UnregisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.UnregisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.UnregisterRequest} UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.UnregisterRequest();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.UnregisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.UnregisterRequest} UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an UnregisterRequest message.
         * @function verify
         * @memberof pb.UnregisterRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        UnregisterRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            return null;
        };

        /**
         * Creates an UnregisterRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.UnregisterRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.UnregisterRequest} UnregisterRequest
         */
        UnregisterRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.UnregisterRequest)
                return object;
            var message = new $root.pb.UnregisterRequest();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            return message;
        };

        /**
         * Creates a plain object from an UnregisterRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.UnregisterRequest} message UnregisterRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        UnregisterRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults)
                object.pid = 0;
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            return object;
        };

        /**
         * Converts this UnregisterRequest to JSON.
         * @function toJSON
         * @memberof pb.UnregisterRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        UnregisterRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for UnregisterRequest
         * @function getTypeUrl
         * @memberof pb.UnregisterRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        UnregisterRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.UnregisterRequest";
        };

        return UnregisterRequest;
    })();

    pb.UnregisterResponse = (function() {

        /**
         * Properties of an UnregisterResponse.
         * @memberof pb
         * @interface IUnregisterResponse
         * @property {boolean|null} [success] UnregisterResponse success
         * @property {string|null} [message] UnregisterResponse message
         */

        /**
         * Constructs a new UnregisterResponse.
         * @memberof pb
         * @classdesc Represents an UnregisterResponse.
         * @implements IUnregisterResponse
         * @constructor
         * @param {pb.IUnregisterResponse=} [properties] Properties to set
         */
        function UnregisterResponse(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * UnregisterResponse success.
         * @member {boolean} success
         * @memberof pb.UnregisterResponse
         * @instance
         */
        UnregisterResponse.prototype.success = false;

        /**
         * UnregisterResponse message.
         * @member {string} message
         * @memberof pb.UnregisterResponse
         * @instance
         */
        UnregisterResponse.prototype.message = "";

        /**
         * Creates a new UnregisterResponse instance using the specified properties.
         * @function create
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse=} [properties] Properties to set
         * @returns {pb.UnregisterResponse} UnregisterResponse instance
         */
        UnregisterResponse.create = function create(properties) {
            return new UnregisterResponse(properties);
        };

        /**
         * Encodes the specified UnregisterResponse message. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse} message UnregisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified UnregisterResponse message, length delimited. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse} message UnregisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.UnregisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.UnregisterResponse} UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.UnregisterResponse();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.UnregisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.UnregisterResponse} UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an UnregisterResponse message.
         * @function verify
         * @memberof pb.UnregisterResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        UnregisterResponse.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates an UnregisterResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.UnregisterResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.UnregisterResponse} UnregisterResponse
         */
        UnregisterResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.UnregisterResponse)
                return object;
            var message = new $root.pb.UnregisterResponse();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from an UnregisterResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.UnregisterResponse} message UnregisterResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        UnregisterResponse.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.success = false;
                object.message = "";
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this UnregisterResponse to JSON.
         * @function toJSON
         * @memberof pb.UnregisterResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        UnregisterResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for UnregisterResponse
         * @function getTypeUrl
         * @memberof pb.UnregisterResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        UnregisterResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.UnregisterResponse";
        };

        return UnregisterResponse;
    })();

    pb.Event = (function() {

        /**
         * Properties of an Event.
         * @memberof pb
         * @interface IEvent
         * @property {number|null} [pid] Event pid
         * @property {string|null} [type] Event type
         * @property {string|null} [comm] Event comm
         * @property {string|null} [path] Event path
         */

        /**
         * Constructs a new Event.
         * @memberof pb
         * @classdesc Represents an Event.
         * @implements IEvent
         * @constructor
         * @param {pb.IEvent=} [properties] Properties to set
         */
        function Event(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Event pid.
         * @member {number} pid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.pid = 0;

        /**
         * Event type.
         * @member {string} type
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.type = "";

        /**
         * Event comm.
         * @member {string} comm
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.comm = "";

        /**
         * Event path.
         * @member {string} path
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.path = "";

        /**
         * Creates a new Event instance using the specified properties.
         * @function create
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent=} [properties] Properties to set
         * @returns {pb.Event} Event instance
         */
        Event.create = function create(properties) {
            return new Event(properties);
        };

        /**
         * Encodes the specified Event message. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @function encode
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent} message Event message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Event.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            if (message.type != null && Object.hasOwnProperty.call(message, "type"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.type);
            if (message.comm != null && Object.hasOwnProperty.call(message, "comm"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.comm);
            if (message.path != null && Object.hasOwnProperty.call(message, "path"))
                writer.uint32(/* id 4, wireType 2 =*/34).string(message.path);
            return writer;
        };

        /**
         * Encodes the specified Event message, length delimited. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent} message Event message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Event.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Event message from the specified reader or buffer.
         * @function decode
         * @memberof pb.Event
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.Event} Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Event.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.Event();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                case 2: {
                        message.type = reader.string();
                        break;
                    }
                case 3: {
                        message.comm = reader.string();
                        break;
                    }
                case 4: {
                        message.path = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an Event message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.Event
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.Event} Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Event.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Event message.
         * @function verify
         * @memberof pb.Event
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Event.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            if (message.type != null && message.hasOwnProperty("type"))
                if (!$util.isString(message.type))
                    return "type: string expected";
            if (message.comm != null && message.hasOwnProperty("comm"))
                if (!$util.isString(message.comm))
                    return "comm: string expected";
            if (message.path != null && message.hasOwnProperty("path"))
                if (!$util.isString(message.path))
                    return "path: string expected";
            return null;
        };

        /**
         * Creates an Event message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.Event
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.Event} Event
         */
        Event.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.Event)
                return object;
            var message = new $root.pb.Event();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            if (object.type != null)
                message.type = String(object.type);
            if (object.comm != null)
                message.comm = String(object.comm);
            if (object.path != null)
                message.path = String(object.path);
            return message;
        };

        /**
         * Creates a plain object from an Event message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.Event
         * @static
         * @param {pb.Event} message Event
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Event.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.pid = 0;
                object.type = "";
                object.comm = "";
                object.path = "";
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.type != null && message.hasOwnProperty("type"))
                object.type = message.type;
            if (message.comm != null && message.hasOwnProperty("comm"))
                object.comm = message.comm;
            if (message.path != null && message.hasOwnProperty("path"))
                object.path = message.path;
            return object;
        };

        /**
         * Converts this Event to JSON.
         * @function toJSON
         * @memberof pb.Event
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Event.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Event
         * @function getTypeUrl
         * @memberof pb.Event
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Event.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.Event";
        };

        return Event;
    })();

    return pb;
})();

module.exports = $root;
