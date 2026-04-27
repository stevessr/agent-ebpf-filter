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

    /**
     * EventType enum.
     * @name pb.EventType
     * @enum {number}
     * @property {number} EXECVE=0 EXECVE value
     * @property {number} OPENAT=1 OPENAT value
     * @property {number} NETWORK_CONNECT=2 NETWORK_CONNECT value
     * @property {number} MKDIR=3 MKDIR value
     * @property {number} UNLINK=4 UNLINK value
     * @property {number} IOCTL=5 IOCTL value
     * @property {number} NETWORK_BIND=6 NETWORK_BIND value
     * @property {number} NETWORK_SENDTO=7 NETWORK_SENDTO value
     * @property {number} NETWORK_RECVFROM=8 NETWORK_RECVFROM value
     * @property {number} READ=9 READ value
     * @property {number} WRITE=10 WRITE value
     * @property {number} OPEN=11 OPEN value
     * @property {number} CHMOD=12 CHMOD value
     * @property {number} CHOWN=13 CHOWN value
     * @property {number} RENAME=14 RENAME value
     * @property {number} LINK=15 LINK value
     * @property {number} SYMLINK=16 SYMLINK value
     * @property {number} MKNOD=17 MKNOD value
     * @property {number} CLONE=18 CLONE value
     * @property {number} EXIT=19 EXIT value
     * @property {number} SOCKET=20 SOCKET value
     * @property {number} ACCEPT=21 ACCEPT value
     * @property {number} ACCEPT4=22 ACCEPT4 value
     * @property {number} WRAPPER_INTERCEPT=23 WRAPPER_INTERCEPT value
     * @property {number} NATIVE_HOOK=24 NATIVE_HOOK value
     */
    pb.EventType = (function() {
        var valuesById = {}, values = Object.create(valuesById);
        values[valuesById[0] = "EXECVE"] = 0;
        values[valuesById[1] = "OPENAT"] = 1;
        values[valuesById[2] = "NETWORK_CONNECT"] = 2;
        values[valuesById[3] = "MKDIR"] = 3;
        values[valuesById[4] = "UNLINK"] = 4;
        values[valuesById[5] = "IOCTL"] = 5;
        values[valuesById[6] = "NETWORK_BIND"] = 6;
        values[valuesById[7] = "NETWORK_SENDTO"] = 7;
        values[valuesById[8] = "NETWORK_RECVFROM"] = 8;
        values[valuesById[9] = "READ"] = 9;
        values[valuesById[10] = "WRITE"] = 10;
        values[valuesById[11] = "OPEN"] = 11;
        values[valuesById[12] = "CHMOD"] = 12;
        values[valuesById[13] = "CHOWN"] = 13;
        values[valuesById[14] = "RENAME"] = 14;
        values[valuesById[15] = "LINK"] = 15;
        values[valuesById[16] = "SYMLINK"] = 16;
        values[valuesById[17] = "MKNOD"] = 17;
        values[valuesById[18] = "CLONE"] = 18;
        values[valuesById[19] = "EXIT"] = 19;
        values[valuesById[20] = "SOCKET"] = 20;
        values[valuesById[21] = "ACCEPT"] = 21;
        values[valuesById[22] = "ACCEPT4"] = 22;
        values[valuesById[23] = "WRAPPER_INTERCEPT"] = 23;
        values[valuesById[24] = "NATIVE_HOOK"] = 24;
        return values;
    })();

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
         * @property {number|null} [ppid] Event ppid
         * @property {number|null} [uid] Event uid
         * @property {string|null} [type] Event type
         * @property {string|null} [tag] Event tag
         * @property {string|null} [comm] Event comm
         * @property {string|null} [path] Event path
         * @property {string|null} [netDirection] Event netDirection
         * @property {string|null} [netEndpoint] Event netEndpoint
         * @property {number|null} [netBytes] Event netBytes
         * @property {string|null} [netFamily] Event netFamily
         * @property {number|Long|null} [retval] Event retval
         * @property {string|null} [extraInfo] Event extraInfo
         * @property {string|null} [extraPath] Event extraPath
         * @property {number|Long|null} [bytes] Event bytes
         * @property {string|null} [mode] Event mode
         * @property {string|null} [domain] Event domain
         * @property {string|null} [sockType] Event sockType
         * @property {number|null} [protocol] Event protocol
         * @property {number|null} [uidArg] Event uidArg
         * @property {number|null} [gidArg] Event gidArg
         * @property {pb.EventType|null} [eventType] Event eventType
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
         * Event ppid.
         * @member {number} ppid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.ppid = 0;

        /**
         * Event uid.
         * @member {number} uid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.uid = 0;

        /**
         * Event type.
         * @member {string} type
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.type = "";

        /**
         * Event tag.
         * @member {string} tag
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.tag = "";

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
         * Event netDirection.
         * @member {string} netDirection
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.netDirection = "";

        /**
         * Event netEndpoint.
         * @member {string} netEndpoint
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.netEndpoint = "";

        /**
         * Event netBytes.
         * @member {number} netBytes
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.netBytes = 0;

        /**
         * Event netFamily.
         * @member {string} netFamily
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.netFamily = "";

        /**
         * Event retval.
         * @member {number|Long} retval
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.retval = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Event extraInfo.
         * @member {string} extraInfo
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.extraInfo = "";

        /**
         * Event extraPath.
         * @member {string} extraPath
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.extraPath = "";

        /**
         * Event bytes.
         * @member {number|Long} bytes
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.bytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Event mode.
         * @member {string} mode
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.mode = "";

        /**
         * Event domain.
         * @member {string} domain
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.domain = "";

        /**
         * Event sockType.
         * @member {string} sockType
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.sockType = "";

        /**
         * Event protocol.
         * @member {number} protocol
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.protocol = 0;

        /**
         * Event uidArg.
         * @member {number} uidArg
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.uidArg = 0;

        /**
         * Event gidArg.
         * @member {number} gidArg
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.gidArg = 0;

        /**
         * Event eventType.
         * @member {pb.EventType} eventType
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.eventType = 0;

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
            if (message.ppid != null && Object.hasOwnProperty.call(message, "ppid"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.ppid);
            if (message.uid != null && Object.hasOwnProperty.call(message, "uid"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.uid);
            if (message.type != null && Object.hasOwnProperty.call(message, "type"))
                writer.uint32(/* id 4, wireType 2 =*/34).string(message.type);
            if (message.tag != null && Object.hasOwnProperty.call(message, "tag"))
                writer.uint32(/* id 5, wireType 2 =*/42).string(message.tag);
            if (message.comm != null && Object.hasOwnProperty.call(message, "comm"))
                writer.uint32(/* id 6, wireType 2 =*/50).string(message.comm);
            if (message.path != null && Object.hasOwnProperty.call(message, "path"))
                writer.uint32(/* id 7, wireType 2 =*/58).string(message.path);
            if (message.netDirection != null && Object.hasOwnProperty.call(message, "netDirection"))
                writer.uint32(/* id 8, wireType 2 =*/66).string(message.netDirection);
            if (message.netEndpoint != null && Object.hasOwnProperty.call(message, "netEndpoint"))
                writer.uint32(/* id 9, wireType 2 =*/74).string(message.netEndpoint);
            if (message.netBytes != null && Object.hasOwnProperty.call(message, "netBytes"))
                writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.netBytes);
            if (message.netFamily != null && Object.hasOwnProperty.call(message, "netFamily"))
                writer.uint32(/* id 11, wireType 2 =*/90).string(message.netFamily);
            if (message.retval != null && Object.hasOwnProperty.call(message, "retval"))
                writer.uint32(/* id 12, wireType 0 =*/96).int64(message.retval);
            if (message.extraInfo != null && Object.hasOwnProperty.call(message, "extraInfo"))
                writer.uint32(/* id 13, wireType 2 =*/106).string(message.extraInfo);
            if (message.extraPath != null && Object.hasOwnProperty.call(message, "extraPath"))
                writer.uint32(/* id 14, wireType 2 =*/114).string(message.extraPath);
            if (message.bytes != null && Object.hasOwnProperty.call(message, "bytes"))
                writer.uint32(/* id 15, wireType 0 =*/120).uint64(message.bytes);
            if (message.mode != null && Object.hasOwnProperty.call(message, "mode"))
                writer.uint32(/* id 16, wireType 2 =*/130).string(message.mode);
            if (message.domain != null && Object.hasOwnProperty.call(message, "domain"))
                writer.uint32(/* id 17, wireType 2 =*/138).string(message.domain);
            if (message.sockType != null && Object.hasOwnProperty.call(message, "sockType"))
                writer.uint32(/* id 18, wireType 2 =*/146).string(message.sockType);
            if (message.protocol != null && Object.hasOwnProperty.call(message, "protocol"))
                writer.uint32(/* id 19, wireType 0 =*/152).uint32(message.protocol);
            if (message.uidArg != null && Object.hasOwnProperty.call(message, "uidArg"))
                writer.uint32(/* id 20, wireType 0 =*/160).uint32(message.uidArg);
            if (message.gidArg != null && Object.hasOwnProperty.call(message, "gidArg"))
                writer.uint32(/* id 21, wireType 0 =*/168).uint32(message.gidArg);
            if (message.eventType != null && Object.hasOwnProperty.call(message, "eventType"))
                writer.uint32(/* id 22, wireType 0 =*/176).int32(message.eventType);
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
                        message.ppid = reader.uint32();
                        break;
                    }
                case 3: {
                        message.uid = reader.uint32();
                        break;
                    }
                case 4: {
                        message.type = reader.string();
                        break;
                    }
                case 5: {
                        message.tag = reader.string();
                        break;
                    }
                case 6: {
                        message.comm = reader.string();
                        break;
                    }
                case 7: {
                        message.path = reader.string();
                        break;
                    }
                case 8: {
                        message.netDirection = reader.string();
                        break;
                    }
                case 9: {
                        message.netEndpoint = reader.string();
                        break;
                    }
                case 10: {
                        message.netBytes = reader.uint32();
                        break;
                    }
                case 11: {
                        message.netFamily = reader.string();
                        break;
                    }
                case 12: {
                        message.retval = reader.int64();
                        break;
                    }
                case 13: {
                        message.extraInfo = reader.string();
                        break;
                    }
                case 14: {
                        message.extraPath = reader.string();
                        break;
                    }
                case 15: {
                        message.bytes = reader.uint64();
                        break;
                    }
                case 16: {
                        message.mode = reader.string();
                        break;
                    }
                case 17: {
                        message.domain = reader.string();
                        break;
                    }
                case 18: {
                        message.sockType = reader.string();
                        break;
                    }
                case 19: {
                        message.protocol = reader.uint32();
                        break;
                    }
                case 20: {
                        message.uidArg = reader.uint32();
                        break;
                    }
                case 21: {
                        message.gidArg = reader.uint32();
                        break;
                    }
                case 22: {
                        message.eventType = reader.int32();
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
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                if (!$util.isInteger(message.ppid))
                    return "ppid: integer expected";
            if (message.uid != null && message.hasOwnProperty("uid"))
                if (!$util.isInteger(message.uid))
                    return "uid: integer expected";
            if (message.type != null && message.hasOwnProperty("type"))
                if (!$util.isString(message.type))
                    return "type: string expected";
            if (message.tag != null && message.hasOwnProperty("tag"))
                if (!$util.isString(message.tag))
                    return "tag: string expected";
            if (message.comm != null && message.hasOwnProperty("comm"))
                if (!$util.isString(message.comm))
                    return "comm: string expected";
            if (message.path != null && message.hasOwnProperty("path"))
                if (!$util.isString(message.path))
                    return "path: string expected";
            if (message.netDirection != null && message.hasOwnProperty("netDirection"))
                if (!$util.isString(message.netDirection))
                    return "netDirection: string expected";
            if (message.netEndpoint != null && message.hasOwnProperty("netEndpoint"))
                if (!$util.isString(message.netEndpoint))
                    return "netEndpoint: string expected";
            if (message.netBytes != null && message.hasOwnProperty("netBytes"))
                if (!$util.isInteger(message.netBytes))
                    return "netBytes: integer expected";
            if (message.netFamily != null && message.hasOwnProperty("netFamily"))
                if (!$util.isString(message.netFamily))
                    return "netFamily: string expected";
            if (message.retval != null && message.hasOwnProperty("retval"))
                if (!$util.isInteger(message.retval) && !(message.retval && $util.isInteger(message.retval.low) && $util.isInteger(message.retval.high)))
                    return "retval: integer|Long expected";
            if (message.extraInfo != null && message.hasOwnProperty("extraInfo"))
                if (!$util.isString(message.extraInfo))
                    return "extraInfo: string expected";
            if (message.extraPath != null && message.hasOwnProperty("extraPath"))
                if (!$util.isString(message.extraPath))
                    return "extraPath: string expected";
            if (message.bytes != null && message.hasOwnProperty("bytes"))
                if (!$util.isInteger(message.bytes) && !(message.bytes && $util.isInteger(message.bytes.low) && $util.isInteger(message.bytes.high)))
                    return "bytes: integer|Long expected";
            if (message.mode != null && message.hasOwnProperty("mode"))
                if (!$util.isString(message.mode))
                    return "mode: string expected";
            if (message.domain != null && message.hasOwnProperty("domain"))
                if (!$util.isString(message.domain))
                    return "domain: string expected";
            if (message.sockType != null && message.hasOwnProperty("sockType"))
                if (!$util.isString(message.sockType))
                    return "sockType: string expected";
            if (message.protocol != null && message.hasOwnProperty("protocol"))
                if (!$util.isInteger(message.protocol))
                    return "protocol: integer expected";
            if (message.uidArg != null && message.hasOwnProperty("uidArg"))
                if (!$util.isInteger(message.uidArg))
                    return "uidArg: integer expected";
            if (message.gidArg != null && message.hasOwnProperty("gidArg"))
                if (!$util.isInteger(message.gidArg))
                    return "gidArg: integer expected";
            if (message.eventType != null && message.hasOwnProperty("eventType"))
                switch (message.eventType) {
                default:
                    return "eventType: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                case 4:
                case 5:
                case 6:
                case 7:
                case 8:
                case 9:
                case 10:
                case 11:
                case 12:
                case 13:
                case 14:
                case 15:
                case 16:
                case 17:
                case 18:
                case 19:
                case 20:
                case 21:
                case 22:
                case 23:
                case 24:
                    break;
                }
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
            if (object.ppid != null)
                message.ppid = object.ppid >>> 0;
            if (object.uid != null)
                message.uid = object.uid >>> 0;
            if (object.type != null)
                message.type = String(object.type);
            if (object.tag != null)
                message.tag = String(object.tag);
            if (object.comm != null)
                message.comm = String(object.comm);
            if (object.path != null)
                message.path = String(object.path);
            if (object.netDirection != null)
                message.netDirection = String(object.netDirection);
            if (object.netEndpoint != null)
                message.netEndpoint = String(object.netEndpoint);
            if (object.netBytes != null)
                message.netBytes = object.netBytes >>> 0;
            if (object.netFamily != null)
                message.netFamily = String(object.netFamily);
            if (object.retval != null)
                if ($util.Long)
                    (message.retval = $util.Long.fromValue(object.retval)).unsigned = false;
                else if (typeof object.retval === "string")
                    message.retval = parseInt(object.retval, 10);
                else if (typeof object.retval === "number")
                    message.retval = object.retval;
                else if (typeof object.retval === "object")
                    message.retval = new $util.LongBits(object.retval.low >>> 0, object.retval.high >>> 0).toNumber();
            if (object.extraInfo != null)
                message.extraInfo = String(object.extraInfo);
            if (object.extraPath != null)
                message.extraPath = String(object.extraPath);
            if (object.bytes != null)
                if ($util.Long)
                    (message.bytes = $util.Long.fromValue(object.bytes)).unsigned = true;
                else if (typeof object.bytes === "string")
                    message.bytes = parseInt(object.bytes, 10);
                else if (typeof object.bytes === "number")
                    message.bytes = object.bytes;
                else if (typeof object.bytes === "object")
                    message.bytes = new $util.LongBits(object.bytes.low >>> 0, object.bytes.high >>> 0).toNumber(true);
            if (object.mode != null)
                message.mode = String(object.mode);
            if (object.domain != null)
                message.domain = String(object.domain);
            if (object.sockType != null)
                message.sockType = String(object.sockType);
            if (object.protocol != null)
                message.protocol = object.protocol >>> 0;
            if (object.uidArg != null)
                message.uidArg = object.uidArg >>> 0;
            if (object.gidArg != null)
                message.gidArg = object.gidArg >>> 0;
            switch (object.eventType) {
            default:
                if (typeof object.eventType === "number") {
                    message.eventType = object.eventType;
                    break;
                }
                break;
            case "EXECVE":
            case 0:
                message.eventType = 0;
                break;
            case "OPENAT":
            case 1:
                message.eventType = 1;
                break;
            case "NETWORK_CONNECT":
            case 2:
                message.eventType = 2;
                break;
            case "MKDIR":
            case 3:
                message.eventType = 3;
                break;
            case "UNLINK":
            case 4:
                message.eventType = 4;
                break;
            case "IOCTL":
            case 5:
                message.eventType = 5;
                break;
            case "NETWORK_BIND":
            case 6:
                message.eventType = 6;
                break;
            case "NETWORK_SENDTO":
            case 7:
                message.eventType = 7;
                break;
            case "NETWORK_RECVFROM":
            case 8:
                message.eventType = 8;
                break;
            case "READ":
            case 9:
                message.eventType = 9;
                break;
            case "WRITE":
            case 10:
                message.eventType = 10;
                break;
            case "OPEN":
            case 11:
                message.eventType = 11;
                break;
            case "CHMOD":
            case 12:
                message.eventType = 12;
                break;
            case "CHOWN":
            case 13:
                message.eventType = 13;
                break;
            case "RENAME":
            case 14:
                message.eventType = 14;
                break;
            case "LINK":
            case 15:
                message.eventType = 15;
                break;
            case "SYMLINK":
            case 16:
                message.eventType = 16;
                break;
            case "MKNOD":
            case 17:
                message.eventType = 17;
                break;
            case "CLONE":
            case 18:
                message.eventType = 18;
                break;
            case "EXIT":
            case 19:
                message.eventType = 19;
                break;
            case "SOCKET":
            case 20:
                message.eventType = 20;
                break;
            case "ACCEPT":
            case 21:
                message.eventType = 21;
                break;
            case "ACCEPT4":
            case 22:
                message.eventType = 22;
                break;
            case "WRAPPER_INTERCEPT":
            case 23:
                message.eventType = 23;
                break;
            case "NATIVE_HOOK":
            case 24:
                message.eventType = 24;
                break;
            }
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
                object.ppid = 0;
                object.uid = 0;
                object.type = "";
                object.tag = "";
                object.comm = "";
                object.path = "";
                object.netDirection = "";
                object.netEndpoint = "";
                object.netBytes = 0;
                object.netFamily = "";
                if ($util.Long) {
                    var long = new $util.Long(0, 0, false);
                    object.retval = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.retval = options.longs === String ? "0" : 0;
                object.extraInfo = "";
                object.extraPath = "";
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.bytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.bytes = options.longs === String ? "0" : 0;
                object.mode = "";
                object.domain = "";
                object.sockType = "";
                object.protocol = 0;
                object.uidArg = 0;
                object.gidArg = 0;
                object.eventType = options.enums === String ? "EXECVE" : 0;
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                object.ppid = message.ppid;
            if (message.uid != null && message.hasOwnProperty("uid"))
                object.uid = message.uid;
            if (message.type != null && message.hasOwnProperty("type"))
                object.type = message.type;
            if (message.tag != null && message.hasOwnProperty("tag"))
                object.tag = message.tag;
            if (message.comm != null && message.hasOwnProperty("comm"))
                object.comm = message.comm;
            if (message.path != null && message.hasOwnProperty("path"))
                object.path = message.path;
            if (message.netDirection != null && message.hasOwnProperty("netDirection"))
                object.netDirection = message.netDirection;
            if (message.netEndpoint != null && message.hasOwnProperty("netEndpoint"))
                object.netEndpoint = message.netEndpoint;
            if (message.netBytes != null && message.hasOwnProperty("netBytes"))
                object.netBytes = message.netBytes;
            if (message.netFamily != null && message.hasOwnProperty("netFamily"))
                object.netFamily = message.netFamily;
            if (message.retval != null && message.hasOwnProperty("retval"))
                if (typeof message.retval === "number")
                    object.retval = options.longs === String ? String(message.retval) : message.retval;
                else
                    object.retval = options.longs === String ? $util.Long.prototype.toString.call(message.retval) : options.longs === Number ? new $util.LongBits(message.retval.low >>> 0, message.retval.high >>> 0).toNumber() : message.retval;
            if (message.extraInfo != null && message.hasOwnProperty("extraInfo"))
                object.extraInfo = message.extraInfo;
            if (message.extraPath != null && message.hasOwnProperty("extraPath"))
                object.extraPath = message.extraPath;
            if (message.bytes != null && message.hasOwnProperty("bytes"))
                if (typeof message.bytes === "number")
                    object.bytes = options.longs === String ? String(message.bytes) : message.bytes;
                else
                    object.bytes = options.longs === String ? $util.Long.prototype.toString.call(message.bytes) : options.longs === Number ? new $util.LongBits(message.bytes.low >>> 0, message.bytes.high >>> 0).toNumber(true) : message.bytes;
            if (message.mode != null && message.hasOwnProperty("mode"))
                object.mode = message.mode;
            if (message.domain != null && message.hasOwnProperty("domain"))
                object.domain = message.domain;
            if (message.sockType != null && message.hasOwnProperty("sockType"))
                object.sockType = message.sockType;
            if (message.protocol != null && message.hasOwnProperty("protocol"))
                object.protocol = message.protocol;
            if (message.uidArg != null && message.hasOwnProperty("uidArg"))
                object.uidArg = message.uidArg;
            if (message.gidArg != null && message.hasOwnProperty("gidArg"))
                object.gidArg = message.gidArg;
            if (message.eventType != null && message.hasOwnProperty("eventType"))
                object.eventType = options.enums === String ? $root.pb.EventType[message.eventType] === undefined ? message.eventType : $root.pb.EventType[message.eventType] : message.eventType;
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

    pb.EventBatch = (function() {

        /**
         * Properties of an EventBatch.
         * @memberof pb
         * @interface IEventBatch
         * @property {Array.<pb.IEvent>|null} [events] EventBatch events
         */

        /**
         * Constructs a new EventBatch.
         * @memberof pb
         * @classdesc Represents an EventBatch.
         * @implements IEventBatch
         * @constructor
         * @param {pb.IEventBatch=} [properties] Properties to set
         */
        function EventBatch(properties) {
            this.events = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * EventBatch events.
         * @member {Array.<pb.IEvent>} events
         * @memberof pb.EventBatch
         * @instance
         */
        EventBatch.prototype.events = $util.emptyArray;

        /**
         * Creates a new EventBatch instance using the specified properties.
         * @function create
         * @memberof pb.EventBatch
         * @static
         * @param {pb.IEventBatch=} [properties] Properties to set
         * @returns {pb.EventBatch} EventBatch instance
         */
        EventBatch.create = function create(properties) {
            return new EventBatch(properties);
        };

        /**
         * Encodes the specified EventBatch message. Does not implicitly {@link pb.EventBatch.verify|verify} messages.
         * @function encode
         * @memberof pb.EventBatch
         * @static
         * @param {pb.IEventBatch} message EventBatch message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EventBatch.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.events != null && message.events.length)
                for (var i = 0; i < message.events.length; ++i)
                    $root.pb.Event.encode(message.events[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified EventBatch message, length delimited. Does not implicitly {@link pb.EventBatch.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.EventBatch
         * @static
         * @param {pb.IEventBatch} message EventBatch message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        EventBatch.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an EventBatch message from the specified reader or buffer.
         * @function decode
         * @memberof pb.EventBatch
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.EventBatch} EventBatch
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EventBatch.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.EventBatch();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.events && message.events.length))
                            message.events = [];
                        message.events.push($root.pb.Event.decode(reader, reader.uint32()));
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
         * Decodes an EventBatch message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.EventBatch
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.EventBatch} EventBatch
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        EventBatch.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an EventBatch message.
         * @function verify
         * @memberof pb.EventBatch
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        EventBatch.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.events != null && message.hasOwnProperty("events")) {
                if (!Array.isArray(message.events))
                    return "events: array expected";
                for (var i = 0; i < message.events.length; ++i) {
                    var error = $root.pb.Event.verify(message.events[i]);
                    if (error)
                        return "events." + error;
                }
            }
            return null;
        };

        /**
         * Creates an EventBatch message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.EventBatch
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.EventBatch} EventBatch
         */
        EventBatch.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.EventBatch)
                return object;
            var message = new $root.pb.EventBatch();
            if (object.events) {
                if (!Array.isArray(object.events))
                    throw TypeError(".pb.EventBatch.events: array expected");
                message.events = [];
                for (var i = 0; i < object.events.length; ++i) {
                    if (typeof object.events[i] !== "object")
                        throw TypeError(".pb.EventBatch.events: object expected");
                    message.events[i] = $root.pb.Event.fromObject(object.events[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from an EventBatch message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.EventBatch
         * @static
         * @param {pb.EventBatch} message EventBatch
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        EventBatch.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults)
                object.events = [];
            if (message.events && message.events.length) {
                object.events = [];
                for (var j = 0; j < message.events.length; ++j)
                    object.events[j] = $root.pb.Event.toObject(message.events[j], options);
            }
            return object;
        };

        /**
         * Converts this EventBatch to JSON.
         * @function toJSON
         * @memberof pb.EventBatch
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        EventBatch.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for EventBatch
         * @function getTypeUrl
         * @memberof pb.EventBatch
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        EventBatch.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.EventBatch";
        };

        return EventBatch;
    })();

    pb.Process = (function() {

        /**
         * Properties of a Process.
         * @memberof pb
         * @interface IProcess
         * @property {number|null} [pid] Process pid
         * @property {number|null} [ppid] Process ppid
         * @property {string|null} [name] Process name
         * @property {number|null} [cpu] Process cpu
         * @property {number|null} [mem] Process mem
         * @property {string|null} [user] Process user
         * @property {number|null} [gpuMem] Process gpuMem
         * @property {number|null} [gpuUtil] Process gpuUtil
         * @property {number|null} [gpuId] Process gpuId
         * @property {string|null} [cmdline] Process cmdline
         * @property {number|Long|null} [createTime] Process createTime
         * @property {number|Long|null} [minorFaults] Process minorFaults
         * @property {number|Long|null} [majorFaults] Process majorFaults
         */

        /**
         * Constructs a new Process.
         * @memberof pb
         * @classdesc Represents a Process.
         * @implements IProcess
         * @constructor
         * @param {pb.IProcess=} [properties] Properties to set
         */
        function Process(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Process pid.
         * @member {number} pid
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.pid = 0;

        /**
         * Process ppid.
         * @member {number} ppid
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.ppid = 0;

        /**
         * Process name.
         * @member {string} name
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.name = "";

        /**
         * Process cpu.
         * @member {number} cpu
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.cpu = 0;

        /**
         * Process mem.
         * @member {number} mem
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.mem = 0;

        /**
         * Process user.
         * @member {string} user
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.user = "";

        /**
         * Process gpuMem.
         * @member {number} gpuMem
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.gpuMem = 0;

        /**
         * Process gpuUtil.
         * @member {number} gpuUtil
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.gpuUtil = 0;

        /**
         * Process gpuId.
         * @member {number} gpuId
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.gpuId = 0;

        /**
         * Process cmdline.
         * @member {string} cmdline
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.cmdline = "";

        /**
         * Process createTime.
         * @member {number|Long} createTime
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.createTime = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

        /**
         * Process minorFaults.
         * @member {number|Long} minorFaults
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.minorFaults = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Process majorFaults.
         * @member {number|Long} majorFaults
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.majorFaults = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new Process instance using the specified properties.
         * @function create
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess=} [properties] Properties to set
         * @returns {pb.Process} Process instance
         */
        Process.create = function create(properties) {
            return new Process(properties);
        };

        /**
         * Encodes the specified Process message. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @function encode
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess} message Process message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Process.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.pid);
            if (message.ppid != null && Object.hasOwnProperty.call(message, "ppid"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.ppid);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.name);
            if (message.cpu != null && Object.hasOwnProperty.call(message, "cpu"))
                writer.uint32(/* id 4, wireType 1 =*/33).double(message.cpu);
            if (message.mem != null && Object.hasOwnProperty.call(message, "mem"))
                writer.uint32(/* id 5, wireType 5 =*/45).float(message.mem);
            if (message.user != null && Object.hasOwnProperty.call(message, "user"))
                writer.uint32(/* id 6, wireType 2 =*/50).string(message.user);
            if (message.gpuMem != null && Object.hasOwnProperty.call(message, "gpuMem"))
                writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.gpuMem);
            if (message.gpuUtil != null && Object.hasOwnProperty.call(message, "gpuUtil"))
                writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.gpuUtil);
            if (message.gpuId != null && Object.hasOwnProperty.call(message, "gpuId"))
                writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.gpuId);
            if (message.cmdline != null && Object.hasOwnProperty.call(message, "cmdline"))
                writer.uint32(/* id 10, wireType 2 =*/82).string(message.cmdline);
            if (message.createTime != null && Object.hasOwnProperty.call(message, "createTime"))
                writer.uint32(/* id 11, wireType 0 =*/88).int64(message.createTime);
            if (message.minorFaults != null && Object.hasOwnProperty.call(message, "minorFaults"))
                writer.uint32(/* id 12, wireType 0 =*/96).uint64(message.minorFaults);
            if (message.majorFaults != null && Object.hasOwnProperty.call(message, "majorFaults"))
                writer.uint32(/* id 13, wireType 0 =*/104).uint64(message.majorFaults);
            return writer;
        };

        /**
         * Encodes the specified Process message, length delimited. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess} message Process message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Process.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Process message from the specified reader or buffer.
         * @function decode
         * @memberof pb.Process
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.Process} Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Process.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.Process();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.int32();
                        break;
                    }
                case 2: {
                        message.ppid = reader.int32();
                        break;
                    }
                case 3: {
                        message.name = reader.string();
                        break;
                    }
                case 4: {
                        message.cpu = reader.double();
                        break;
                    }
                case 5: {
                        message.mem = reader.float();
                        break;
                    }
                case 6: {
                        message.user = reader.string();
                        break;
                    }
                case 7: {
                        message.gpuMem = reader.uint32();
                        break;
                    }
                case 8: {
                        message.gpuUtil = reader.uint32();
                        break;
                    }
                case 9: {
                        message.gpuId = reader.uint32();
                        break;
                    }
                case 10: {
                        message.cmdline = reader.string();
                        break;
                    }
                case 11: {
                        message.createTime = reader.int64();
                        break;
                    }
                case 12: {
                        message.minorFaults = reader.uint64();
                        break;
                    }
                case 13: {
                        message.majorFaults = reader.uint64();
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
         * Decodes a Process message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.Process
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.Process} Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Process.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Process message.
         * @function verify
         * @memberof pb.Process
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Process.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                if (!$util.isInteger(message.ppid))
                    return "ppid: integer expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.cpu != null && message.hasOwnProperty("cpu"))
                if (typeof message.cpu !== "number")
                    return "cpu: number expected";
            if (message.mem != null && message.hasOwnProperty("mem"))
                if (typeof message.mem !== "number")
                    return "mem: number expected";
            if (message.user != null && message.hasOwnProperty("user"))
                if (!$util.isString(message.user))
                    return "user: string expected";
            if (message.gpuMem != null && message.hasOwnProperty("gpuMem"))
                if (!$util.isInteger(message.gpuMem))
                    return "gpuMem: integer expected";
            if (message.gpuUtil != null && message.hasOwnProperty("gpuUtil"))
                if (!$util.isInteger(message.gpuUtil))
                    return "gpuUtil: integer expected";
            if (message.gpuId != null && message.hasOwnProperty("gpuId"))
                if (!$util.isInteger(message.gpuId))
                    return "gpuId: integer expected";
            if (message.cmdline != null && message.hasOwnProperty("cmdline"))
                if (!$util.isString(message.cmdline))
                    return "cmdline: string expected";
            if (message.createTime != null && message.hasOwnProperty("createTime"))
                if (!$util.isInteger(message.createTime) && !(message.createTime && $util.isInteger(message.createTime.low) && $util.isInteger(message.createTime.high)))
                    return "createTime: integer|Long expected";
            if (message.minorFaults != null && message.hasOwnProperty("minorFaults"))
                if (!$util.isInteger(message.minorFaults) && !(message.minorFaults && $util.isInteger(message.minorFaults.low) && $util.isInteger(message.minorFaults.high)))
                    return "minorFaults: integer|Long expected";
            if (message.majorFaults != null && message.hasOwnProperty("majorFaults"))
                if (!$util.isInteger(message.majorFaults) && !(message.majorFaults && $util.isInteger(message.majorFaults.low) && $util.isInteger(message.majorFaults.high)))
                    return "majorFaults: integer|Long expected";
            return null;
        };

        /**
         * Creates a Process message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.Process
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.Process} Process
         */
        Process.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.Process)
                return object;
            var message = new $root.pb.Process();
            if (object.pid != null)
                message.pid = object.pid | 0;
            if (object.ppid != null)
                message.ppid = object.ppid | 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.cpu != null)
                message.cpu = Number(object.cpu);
            if (object.mem != null)
                message.mem = Number(object.mem);
            if (object.user != null)
                message.user = String(object.user);
            if (object.gpuMem != null)
                message.gpuMem = object.gpuMem >>> 0;
            if (object.gpuUtil != null)
                message.gpuUtil = object.gpuUtil >>> 0;
            if (object.gpuId != null)
                message.gpuId = object.gpuId >>> 0;
            if (object.cmdline != null)
                message.cmdline = String(object.cmdline);
            if (object.createTime != null)
                if ($util.Long)
                    (message.createTime = $util.Long.fromValue(object.createTime)).unsigned = false;
                else if (typeof object.createTime === "string")
                    message.createTime = parseInt(object.createTime, 10);
                else if (typeof object.createTime === "number")
                    message.createTime = object.createTime;
                else if (typeof object.createTime === "object")
                    message.createTime = new $util.LongBits(object.createTime.low >>> 0, object.createTime.high >>> 0).toNumber();
            if (object.minorFaults != null)
                if ($util.Long)
                    (message.minorFaults = $util.Long.fromValue(object.minorFaults)).unsigned = true;
                else if (typeof object.minorFaults === "string")
                    message.minorFaults = parseInt(object.minorFaults, 10);
                else if (typeof object.minorFaults === "number")
                    message.minorFaults = object.minorFaults;
                else if (typeof object.minorFaults === "object")
                    message.minorFaults = new $util.LongBits(object.minorFaults.low >>> 0, object.minorFaults.high >>> 0).toNumber(true);
            if (object.majorFaults != null)
                if ($util.Long)
                    (message.majorFaults = $util.Long.fromValue(object.majorFaults)).unsigned = true;
                else if (typeof object.majorFaults === "string")
                    message.majorFaults = parseInt(object.majorFaults, 10);
                else if (typeof object.majorFaults === "number")
                    message.majorFaults = object.majorFaults;
                else if (typeof object.majorFaults === "object")
                    message.majorFaults = new $util.LongBits(object.majorFaults.low >>> 0, object.majorFaults.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a Process message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.Process
         * @static
         * @param {pb.Process} message Process
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Process.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.pid = 0;
                object.ppid = 0;
                object.name = "";
                object.cpu = 0;
                object.mem = 0;
                object.user = "";
                object.gpuMem = 0;
                object.gpuUtil = 0;
                object.gpuId = 0;
                object.cmdline = "";
                if ($util.Long) {
                    var long = new $util.Long(0, 0, false);
                    object.createTime = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.createTime = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.minorFaults = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.minorFaults = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.majorFaults = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.majorFaults = options.longs === String ? "0" : 0;
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                object.ppid = message.ppid;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.cpu != null && message.hasOwnProperty("cpu"))
                object.cpu = options.json && !isFinite(message.cpu) ? String(message.cpu) : message.cpu;
            if (message.mem != null && message.hasOwnProperty("mem"))
                object.mem = options.json && !isFinite(message.mem) ? String(message.mem) : message.mem;
            if (message.user != null && message.hasOwnProperty("user"))
                object.user = message.user;
            if (message.gpuMem != null && message.hasOwnProperty("gpuMem"))
                object.gpuMem = message.gpuMem;
            if (message.gpuUtil != null && message.hasOwnProperty("gpuUtil"))
                object.gpuUtil = message.gpuUtil;
            if (message.gpuId != null && message.hasOwnProperty("gpuId"))
                object.gpuId = message.gpuId;
            if (message.cmdline != null && message.hasOwnProperty("cmdline"))
                object.cmdline = message.cmdline;
            if (message.createTime != null && message.hasOwnProperty("createTime"))
                if (typeof message.createTime === "number")
                    object.createTime = options.longs === String ? String(message.createTime) : message.createTime;
                else
                    object.createTime = options.longs === String ? $util.Long.prototype.toString.call(message.createTime) : options.longs === Number ? new $util.LongBits(message.createTime.low >>> 0, message.createTime.high >>> 0).toNumber() : message.createTime;
            if (message.minorFaults != null && message.hasOwnProperty("minorFaults"))
                if (typeof message.minorFaults === "number")
                    object.minorFaults = options.longs === String ? String(message.minorFaults) : message.minorFaults;
                else
                    object.minorFaults = options.longs === String ? $util.Long.prototype.toString.call(message.minorFaults) : options.longs === Number ? new $util.LongBits(message.minorFaults.low >>> 0, message.minorFaults.high >>> 0).toNumber(true) : message.minorFaults;
            if (message.majorFaults != null && message.hasOwnProperty("majorFaults"))
                if (typeof message.majorFaults === "number")
                    object.majorFaults = options.longs === String ? String(message.majorFaults) : message.majorFaults;
                else
                    object.majorFaults = options.longs === String ? $util.Long.prototype.toString.call(message.majorFaults) : options.longs === Number ? new $util.LongBits(message.majorFaults.low >>> 0, message.majorFaults.high >>> 0).toNumber(true) : message.majorFaults;
            return object;
        };

        /**
         * Converts this Process to JSON.
         * @function toJSON
         * @memberof pb.Process
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Process.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Process
         * @function getTypeUrl
         * @memberof pb.Process
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Process.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.Process";
        };

        return Process;
    })();

    pb.GPUStatus = (function() {

        /**
         * Properties of a GPUStatus.
         * @memberof pb
         * @interface IGPUStatus
         * @property {number|null} [index] GPUStatus index
         * @property {string|null} [name] GPUStatus name
         * @property {number|null} [utilGpu] GPUStatus utilGpu
         * @property {number|null} [utilMem] GPUStatus utilMem
         * @property {number|null} [memTotal] GPUStatus memTotal
         * @property {number|null} [memUsed] GPUStatus memUsed
         * @property {number|null} [temp] GPUStatus temp
         * @property {number|null} [encUtil] GPUStatus encUtil
         * @property {number|null} [decUtil] GPUStatus decUtil
         * @property {number|null} [smClockMhz] GPUStatus smClockMhz
         * @property {number|null} [memClockMhz] GPUStatus memClockMhz
         * @property {number|null} [gfxClockMhz] GPUStatus gfxClockMhz
         * @property {number|null} [powerW] GPUStatus powerW
         * @property {number|null} [powerLimitW] GPUStatus powerLimitW
         * @property {number|null} [fanSpeed] GPUStatus fanSpeed
         * @property {number|null} [pcieGen] GPUStatus pcieGen
         * @property {number|null} [pcieWidth] GPUStatus pcieWidth
         */

        /**
         * Constructs a new GPUStatus.
         * @memberof pb
         * @classdesc Represents a GPUStatus.
         * @implements IGPUStatus
         * @constructor
         * @param {pb.IGPUStatus=} [properties] Properties to set
         */
        function GPUStatus(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * GPUStatus index.
         * @member {number} index
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.index = 0;

        /**
         * GPUStatus name.
         * @member {string} name
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.name = "";

        /**
         * GPUStatus utilGpu.
         * @member {number} utilGpu
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.utilGpu = 0;

        /**
         * GPUStatus utilMem.
         * @member {number} utilMem
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.utilMem = 0;

        /**
         * GPUStatus memTotal.
         * @member {number} memTotal
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.memTotal = 0;

        /**
         * GPUStatus memUsed.
         * @member {number} memUsed
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.memUsed = 0;

        /**
         * GPUStatus temp.
         * @member {number} temp
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.temp = 0;

        /**
         * GPUStatus encUtil.
         * @member {number} encUtil
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.encUtil = 0;

        /**
         * GPUStatus decUtil.
         * @member {number} decUtil
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.decUtil = 0;

        /**
         * GPUStatus smClockMhz.
         * @member {number} smClockMhz
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.smClockMhz = 0;

        /**
         * GPUStatus memClockMhz.
         * @member {number} memClockMhz
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.memClockMhz = 0;

        /**
         * GPUStatus gfxClockMhz.
         * @member {number} gfxClockMhz
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.gfxClockMhz = 0;

        /**
         * GPUStatus powerW.
         * @member {number} powerW
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.powerW = 0;

        /**
         * GPUStatus powerLimitW.
         * @member {number} powerLimitW
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.powerLimitW = 0;

        /**
         * GPUStatus fanSpeed.
         * @member {number} fanSpeed
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.fanSpeed = 0;

        /**
         * GPUStatus pcieGen.
         * @member {number} pcieGen
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.pcieGen = 0;

        /**
         * GPUStatus pcieWidth.
         * @member {number} pcieWidth
         * @memberof pb.GPUStatus
         * @instance
         */
        GPUStatus.prototype.pcieWidth = 0;

        /**
         * Creates a new GPUStatus instance using the specified properties.
         * @function create
         * @memberof pb.GPUStatus
         * @static
         * @param {pb.IGPUStatus=} [properties] Properties to set
         * @returns {pb.GPUStatus} GPUStatus instance
         */
        GPUStatus.create = function create(properties) {
            return new GPUStatus(properties);
        };

        /**
         * Encodes the specified GPUStatus message. Does not implicitly {@link pb.GPUStatus.verify|verify} messages.
         * @function encode
         * @memberof pb.GPUStatus
         * @static
         * @param {pb.IGPUStatus} message GPUStatus message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GPUStatus.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.index != null && Object.hasOwnProperty.call(message, "index"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.index);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.utilGpu != null && Object.hasOwnProperty.call(message, "utilGpu"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.utilGpu);
            if (message.utilMem != null && Object.hasOwnProperty.call(message, "utilMem"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.utilMem);
            if (message.memTotal != null && Object.hasOwnProperty.call(message, "memTotal"))
                writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.memTotal);
            if (message.memUsed != null && Object.hasOwnProperty.call(message, "memUsed"))
                writer.uint32(/* id 6, wireType 0 =*/48).uint32(message.memUsed);
            if (message.temp != null && Object.hasOwnProperty.call(message, "temp"))
                writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.temp);
            if (message.encUtil != null && Object.hasOwnProperty.call(message, "encUtil"))
                writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.encUtil);
            if (message.decUtil != null && Object.hasOwnProperty.call(message, "decUtil"))
                writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.decUtil);
            if (message.smClockMhz != null && Object.hasOwnProperty.call(message, "smClockMhz"))
                writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.smClockMhz);
            if (message.memClockMhz != null && Object.hasOwnProperty.call(message, "memClockMhz"))
                writer.uint32(/* id 11, wireType 0 =*/88).uint32(message.memClockMhz);
            if (message.gfxClockMhz != null && Object.hasOwnProperty.call(message, "gfxClockMhz"))
                writer.uint32(/* id 12, wireType 0 =*/96).uint32(message.gfxClockMhz);
            if (message.powerW != null && Object.hasOwnProperty.call(message, "powerW"))
                writer.uint32(/* id 13, wireType 0 =*/104).uint32(message.powerW);
            if (message.powerLimitW != null && Object.hasOwnProperty.call(message, "powerLimitW"))
                writer.uint32(/* id 14, wireType 0 =*/112).uint32(message.powerLimitW);
            if (message.fanSpeed != null && Object.hasOwnProperty.call(message, "fanSpeed"))
                writer.uint32(/* id 15, wireType 0 =*/120).uint32(message.fanSpeed);
            if (message.pcieGen != null && Object.hasOwnProperty.call(message, "pcieGen"))
                writer.uint32(/* id 16, wireType 0 =*/128).int32(message.pcieGen);
            if (message.pcieWidth != null && Object.hasOwnProperty.call(message, "pcieWidth"))
                writer.uint32(/* id 17, wireType 0 =*/136).int32(message.pcieWidth);
            return writer;
        };

        /**
         * Encodes the specified GPUStatus message, length delimited. Does not implicitly {@link pb.GPUStatus.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.GPUStatus
         * @static
         * @param {pb.IGPUStatus} message GPUStatus message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        GPUStatus.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a GPUStatus message from the specified reader or buffer.
         * @function decode
         * @memberof pb.GPUStatus
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.GPUStatus} GPUStatus
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GPUStatus.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.GPUStatus();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.index = reader.uint32();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.utilGpu = reader.uint32();
                        break;
                    }
                case 4: {
                        message.utilMem = reader.uint32();
                        break;
                    }
                case 5: {
                        message.memTotal = reader.uint32();
                        break;
                    }
                case 6: {
                        message.memUsed = reader.uint32();
                        break;
                    }
                case 7: {
                        message.temp = reader.uint32();
                        break;
                    }
                case 8: {
                        message.encUtil = reader.uint32();
                        break;
                    }
                case 9: {
                        message.decUtil = reader.uint32();
                        break;
                    }
                case 10: {
                        message.smClockMhz = reader.uint32();
                        break;
                    }
                case 11: {
                        message.memClockMhz = reader.uint32();
                        break;
                    }
                case 12: {
                        message.gfxClockMhz = reader.uint32();
                        break;
                    }
                case 13: {
                        message.powerW = reader.uint32();
                        break;
                    }
                case 14: {
                        message.powerLimitW = reader.uint32();
                        break;
                    }
                case 15: {
                        message.fanSpeed = reader.uint32();
                        break;
                    }
                case 16: {
                        message.pcieGen = reader.int32();
                        break;
                    }
                case 17: {
                        message.pcieWidth = reader.int32();
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
         * Decodes a GPUStatus message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.GPUStatus
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.GPUStatus} GPUStatus
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        GPUStatus.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a GPUStatus message.
         * @function verify
         * @memberof pb.GPUStatus
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        GPUStatus.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.index != null && message.hasOwnProperty("index"))
                if (!$util.isInteger(message.index))
                    return "index: integer expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.utilGpu != null && message.hasOwnProperty("utilGpu"))
                if (!$util.isInteger(message.utilGpu))
                    return "utilGpu: integer expected";
            if (message.utilMem != null && message.hasOwnProperty("utilMem"))
                if (!$util.isInteger(message.utilMem))
                    return "utilMem: integer expected";
            if (message.memTotal != null && message.hasOwnProperty("memTotal"))
                if (!$util.isInteger(message.memTotal))
                    return "memTotal: integer expected";
            if (message.memUsed != null && message.hasOwnProperty("memUsed"))
                if (!$util.isInteger(message.memUsed))
                    return "memUsed: integer expected";
            if (message.temp != null && message.hasOwnProperty("temp"))
                if (!$util.isInteger(message.temp))
                    return "temp: integer expected";
            if (message.encUtil != null && message.hasOwnProperty("encUtil"))
                if (!$util.isInteger(message.encUtil))
                    return "encUtil: integer expected";
            if (message.decUtil != null && message.hasOwnProperty("decUtil"))
                if (!$util.isInteger(message.decUtil))
                    return "decUtil: integer expected";
            if (message.smClockMhz != null && message.hasOwnProperty("smClockMhz"))
                if (!$util.isInteger(message.smClockMhz))
                    return "smClockMhz: integer expected";
            if (message.memClockMhz != null && message.hasOwnProperty("memClockMhz"))
                if (!$util.isInteger(message.memClockMhz))
                    return "memClockMhz: integer expected";
            if (message.gfxClockMhz != null && message.hasOwnProperty("gfxClockMhz"))
                if (!$util.isInteger(message.gfxClockMhz))
                    return "gfxClockMhz: integer expected";
            if (message.powerW != null && message.hasOwnProperty("powerW"))
                if (!$util.isInteger(message.powerW))
                    return "powerW: integer expected";
            if (message.powerLimitW != null && message.hasOwnProperty("powerLimitW"))
                if (!$util.isInteger(message.powerLimitW))
                    return "powerLimitW: integer expected";
            if (message.fanSpeed != null && message.hasOwnProperty("fanSpeed"))
                if (!$util.isInteger(message.fanSpeed))
                    return "fanSpeed: integer expected";
            if (message.pcieGen != null && message.hasOwnProperty("pcieGen"))
                if (!$util.isInteger(message.pcieGen))
                    return "pcieGen: integer expected";
            if (message.pcieWidth != null && message.hasOwnProperty("pcieWidth"))
                if (!$util.isInteger(message.pcieWidth))
                    return "pcieWidth: integer expected";
            return null;
        };

        /**
         * Creates a GPUStatus message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.GPUStatus
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.GPUStatus} GPUStatus
         */
        GPUStatus.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.GPUStatus)
                return object;
            var message = new $root.pb.GPUStatus();
            if (object.index != null)
                message.index = object.index >>> 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.utilGpu != null)
                message.utilGpu = object.utilGpu >>> 0;
            if (object.utilMem != null)
                message.utilMem = object.utilMem >>> 0;
            if (object.memTotal != null)
                message.memTotal = object.memTotal >>> 0;
            if (object.memUsed != null)
                message.memUsed = object.memUsed >>> 0;
            if (object.temp != null)
                message.temp = object.temp >>> 0;
            if (object.encUtil != null)
                message.encUtil = object.encUtil >>> 0;
            if (object.decUtil != null)
                message.decUtil = object.decUtil >>> 0;
            if (object.smClockMhz != null)
                message.smClockMhz = object.smClockMhz >>> 0;
            if (object.memClockMhz != null)
                message.memClockMhz = object.memClockMhz >>> 0;
            if (object.gfxClockMhz != null)
                message.gfxClockMhz = object.gfxClockMhz >>> 0;
            if (object.powerW != null)
                message.powerW = object.powerW >>> 0;
            if (object.powerLimitW != null)
                message.powerLimitW = object.powerLimitW >>> 0;
            if (object.fanSpeed != null)
                message.fanSpeed = object.fanSpeed >>> 0;
            if (object.pcieGen != null)
                message.pcieGen = object.pcieGen | 0;
            if (object.pcieWidth != null)
                message.pcieWidth = object.pcieWidth | 0;
            return message;
        };

        /**
         * Creates a plain object from a GPUStatus message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.GPUStatus
         * @static
         * @param {pb.GPUStatus} message GPUStatus
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        GPUStatus.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.index = 0;
                object.name = "";
                object.utilGpu = 0;
                object.utilMem = 0;
                object.memTotal = 0;
                object.memUsed = 0;
                object.temp = 0;
                object.encUtil = 0;
                object.decUtil = 0;
                object.smClockMhz = 0;
                object.memClockMhz = 0;
                object.gfxClockMhz = 0;
                object.powerW = 0;
                object.powerLimitW = 0;
                object.fanSpeed = 0;
                object.pcieGen = 0;
                object.pcieWidth = 0;
            }
            if (message.index != null && message.hasOwnProperty("index"))
                object.index = message.index;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.utilGpu != null && message.hasOwnProperty("utilGpu"))
                object.utilGpu = message.utilGpu;
            if (message.utilMem != null && message.hasOwnProperty("utilMem"))
                object.utilMem = message.utilMem;
            if (message.memTotal != null && message.hasOwnProperty("memTotal"))
                object.memTotal = message.memTotal;
            if (message.memUsed != null && message.hasOwnProperty("memUsed"))
                object.memUsed = message.memUsed;
            if (message.temp != null && message.hasOwnProperty("temp"))
                object.temp = message.temp;
            if (message.encUtil != null && message.hasOwnProperty("encUtil"))
                object.encUtil = message.encUtil;
            if (message.decUtil != null && message.hasOwnProperty("decUtil"))
                object.decUtil = message.decUtil;
            if (message.smClockMhz != null && message.hasOwnProperty("smClockMhz"))
                object.smClockMhz = message.smClockMhz;
            if (message.memClockMhz != null && message.hasOwnProperty("memClockMhz"))
                object.memClockMhz = message.memClockMhz;
            if (message.gfxClockMhz != null && message.hasOwnProperty("gfxClockMhz"))
                object.gfxClockMhz = message.gfxClockMhz;
            if (message.powerW != null && message.hasOwnProperty("powerW"))
                object.powerW = message.powerW;
            if (message.powerLimitW != null && message.hasOwnProperty("powerLimitW"))
                object.powerLimitW = message.powerLimitW;
            if (message.fanSpeed != null && message.hasOwnProperty("fanSpeed"))
                object.fanSpeed = message.fanSpeed;
            if (message.pcieGen != null && message.hasOwnProperty("pcieGen"))
                object.pcieGen = message.pcieGen;
            if (message.pcieWidth != null && message.hasOwnProperty("pcieWidth"))
                object.pcieWidth = message.pcieWidth;
            return object;
        };

        /**
         * Converts this GPUStatus to JSON.
         * @function toJSON
         * @memberof pb.GPUStatus
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        GPUStatus.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for GPUStatus
         * @function getTypeUrl
         * @memberof pb.GPUStatus
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        GPUStatus.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.GPUStatus";
        };

        return GPUStatus;
    })();

    pb.CPUInfo = (function() {

        /**
         * Properties of a CPUInfo.
         * @memberof pb
         * @interface ICPUInfo
         * @property {number|null} [total] CPUInfo total
         * @property {Array.<number>|null} [cores] CPUInfo cores
         * @property {Array.<pb.CPUInfo.ICore>|null} [coreDetails] CPUInfo coreDetails
         */

        /**
         * Constructs a new CPUInfo.
         * @memberof pb
         * @classdesc Represents a CPUInfo.
         * @implements ICPUInfo
         * @constructor
         * @param {pb.ICPUInfo=} [properties] Properties to set
         */
        function CPUInfo(properties) {
            this.cores = [];
            this.coreDetails = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * CPUInfo total.
         * @member {number} total
         * @memberof pb.CPUInfo
         * @instance
         */
        CPUInfo.prototype.total = 0;

        /**
         * CPUInfo cores.
         * @member {Array.<number>} cores
         * @memberof pb.CPUInfo
         * @instance
         */
        CPUInfo.prototype.cores = $util.emptyArray;

        /**
         * CPUInfo coreDetails.
         * @member {Array.<pb.CPUInfo.ICore>} coreDetails
         * @memberof pb.CPUInfo
         * @instance
         */
        CPUInfo.prototype.coreDetails = $util.emptyArray;

        /**
         * Creates a new CPUInfo instance using the specified properties.
         * @function create
         * @memberof pb.CPUInfo
         * @static
         * @param {pb.ICPUInfo=} [properties] Properties to set
         * @returns {pb.CPUInfo} CPUInfo instance
         */
        CPUInfo.create = function create(properties) {
            return new CPUInfo(properties);
        };

        /**
         * Encodes the specified CPUInfo message. Does not implicitly {@link pb.CPUInfo.verify|verify} messages.
         * @function encode
         * @memberof pb.CPUInfo
         * @static
         * @param {pb.ICPUInfo} message CPUInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        CPUInfo.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.total != null && Object.hasOwnProperty.call(message, "total"))
                writer.uint32(/* id 1, wireType 1 =*/9).double(message.total);
            if (message.cores != null && message.cores.length) {
                writer.uint32(/* id 2, wireType 2 =*/18).fork();
                for (var i = 0; i < message.cores.length; ++i)
                    writer.double(message.cores[i]);
                writer.ldelim();
            }
            if (message.coreDetails != null && message.coreDetails.length)
                for (var i = 0; i < message.coreDetails.length; ++i)
                    $root.pb.CPUInfo.Core.encode(message.coreDetails[i], writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified CPUInfo message, length delimited. Does not implicitly {@link pb.CPUInfo.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.CPUInfo
         * @static
         * @param {pb.ICPUInfo} message CPUInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        CPUInfo.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a CPUInfo message from the specified reader or buffer.
         * @function decode
         * @memberof pb.CPUInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.CPUInfo} CPUInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        CPUInfo.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.CPUInfo();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.total = reader.double();
                        break;
                    }
                case 2: {
                        if (!(message.cores && message.cores.length))
                            message.cores = [];
                        if ((tag & 7) === 2) {
                            var end2 = reader.uint32() + reader.pos;
                            while (reader.pos < end2)
                                message.cores.push(reader.double());
                        } else
                            message.cores.push(reader.double());
                        break;
                    }
                case 3: {
                        if (!(message.coreDetails && message.coreDetails.length))
                            message.coreDetails = [];
                        message.coreDetails.push($root.pb.CPUInfo.Core.decode(reader, reader.uint32()));
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
         * Decodes a CPUInfo message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.CPUInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.CPUInfo} CPUInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        CPUInfo.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a CPUInfo message.
         * @function verify
         * @memberof pb.CPUInfo
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        CPUInfo.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.total != null && message.hasOwnProperty("total"))
                if (typeof message.total !== "number")
                    return "total: number expected";
            if (message.cores != null && message.hasOwnProperty("cores")) {
                if (!Array.isArray(message.cores))
                    return "cores: array expected";
                for (var i = 0; i < message.cores.length; ++i)
                    if (typeof message.cores[i] !== "number")
                        return "cores: number[] expected";
            }
            if (message.coreDetails != null && message.hasOwnProperty("coreDetails")) {
                if (!Array.isArray(message.coreDetails))
                    return "coreDetails: array expected";
                for (var i = 0; i < message.coreDetails.length; ++i) {
                    var error = $root.pb.CPUInfo.Core.verify(message.coreDetails[i]);
                    if (error)
                        return "coreDetails." + error;
                }
            }
            return null;
        };

        /**
         * Creates a CPUInfo message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.CPUInfo
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.CPUInfo} CPUInfo
         */
        CPUInfo.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.CPUInfo)
                return object;
            var message = new $root.pb.CPUInfo();
            if (object.total != null)
                message.total = Number(object.total);
            if (object.cores) {
                if (!Array.isArray(object.cores))
                    throw TypeError(".pb.CPUInfo.cores: array expected");
                message.cores = [];
                for (var i = 0; i < object.cores.length; ++i)
                    message.cores[i] = Number(object.cores[i]);
            }
            if (object.coreDetails) {
                if (!Array.isArray(object.coreDetails))
                    throw TypeError(".pb.CPUInfo.coreDetails: array expected");
                message.coreDetails = [];
                for (var i = 0; i < object.coreDetails.length; ++i) {
                    if (typeof object.coreDetails[i] !== "object")
                        throw TypeError(".pb.CPUInfo.coreDetails: object expected");
                    message.coreDetails[i] = $root.pb.CPUInfo.Core.fromObject(object.coreDetails[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a CPUInfo message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.CPUInfo
         * @static
         * @param {pb.CPUInfo} message CPUInfo
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        CPUInfo.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults) {
                object.cores = [];
                object.coreDetails = [];
            }
            if (options.defaults)
                object.total = 0;
            if (message.total != null && message.hasOwnProperty("total"))
                object.total = options.json && !isFinite(message.total) ? String(message.total) : message.total;
            if (message.cores && message.cores.length) {
                object.cores = [];
                for (var j = 0; j < message.cores.length; ++j)
                    object.cores[j] = options.json && !isFinite(message.cores[j]) ? String(message.cores[j]) : message.cores[j];
            }
            if (message.coreDetails && message.coreDetails.length) {
                object.coreDetails = [];
                for (var j = 0; j < message.coreDetails.length; ++j)
                    object.coreDetails[j] = $root.pb.CPUInfo.Core.toObject(message.coreDetails[j], options);
            }
            return object;
        };

        /**
         * Converts this CPUInfo to JSON.
         * @function toJSON
         * @memberof pb.CPUInfo
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        CPUInfo.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for CPUInfo
         * @function getTypeUrl
         * @memberof pb.CPUInfo
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        CPUInfo.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.CPUInfo";
        };

        CPUInfo.Core = (function() {

            /**
             * Properties of a Core.
             * @memberof pb.CPUInfo
             * @interface ICore
             * @property {number|null} [index] Core index
             * @property {number|null} [usage] Core usage
             * @property {pb.CPUInfo.Core.Type|null} [type] Core type
             */

            /**
             * Constructs a new Core.
             * @memberof pb.CPUInfo
             * @classdesc Represents a Core.
             * @implements ICore
             * @constructor
             * @param {pb.CPUInfo.ICore=} [properties] Properties to set
             */
            function Core(properties) {
                if (properties)
                    for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null)
                            this[keys[i]] = properties[keys[i]];
            }

            /**
             * Core index.
             * @member {number} index
             * @memberof pb.CPUInfo.Core
             * @instance
             */
            Core.prototype.index = 0;

            /**
             * Core usage.
             * @member {number} usage
             * @memberof pb.CPUInfo.Core
             * @instance
             */
            Core.prototype.usage = 0;

            /**
             * Core type.
             * @member {pb.CPUInfo.Core.Type} type
             * @memberof pb.CPUInfo.Core
             * @instance
             */
            Core.prototype.type = 0;

            /**
             * Creates a new Core instance using the specified properties.
             * @function create
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {pb.CPUInfo.ICore=} [properties] Properties to set
             * @returns {pb.CPUInfo.Core} Core instance
             */
            Core.create = function create(properties) {
                return new Core(properties);
            };

            /**
             * Encodes the specified Core message. Does not implicitly {@link pb.CPUInfo.Core.verify|verify} messages.
             * @function encode
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {pb.CPUInfo.ICore} message Core message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Core.encode = function encode(message, writer) {
                if (!writer)
                    writer = $Writer.create();
                if (message.index != null && Object.hasOwnProperty.call(message, "index"))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.index);
                if (message.usage != null && Object.hasOwnProperty.call(message, "usage"))
                    writer.uint32(/* id 2, wireType 1 =*/17).double(message.usage);
                if (message.type != null && Object.hasOwnProperty.call(message, "type"))
                    writer.uint32(/* id 3, wireType 0 =*/24).int32(message.type);
                return writer;
            };

            /**
             * Encodes the specified Core message, length delimited. Does not implicitly {@link pb.CPUInfo.Core.verify|verify} messages.
             * @function encodeDelimited
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {pb.CPUInfo.ICore} message Core message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Core.encodeDelimited = function encodeDelimited(message, writer) {
                return this.encode(message, writer).ldelim();
            };

            /**
             * Decodes a Core message from the specified reader or buffer.
             * @function decode
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {pb.CPUInfo.Core} Core
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Core.decode = function decode(reader, length, error) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.CPUInfo.Core();
                while (reader.pos < end) {
                    var tag = reader.uint32();
                    if (tag === error)
                        break;
                    switch (tag >>> 3) {
                    case 1: {
                            message.index = reader.uint32();
                            break;
                        }
                    case 2: {
                            message.usage = reader.double();
                            break;
                        }
                    case 3: {
                            message.type = reader.int32();
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
             * Decodes a Core message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {pb.CPUInfo.Core} Core
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Core.decodeDelimited = function decodeDelimited(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.uint32());
            };

            /**
             * Verifies a Core message.
             * @function verify
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            Core.verify = function verify(message) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (message.index != null && message.hasOwnProperty("index"))
                    if (!$util.isInteger(message.index))
                        return "index: integer expected";
                if (message.usage != null && message.hasOwnProperty("usage"))
                    if (typeof message.usage !== "number")
                        return "usage: number expected";
                if (message.type != null && message.hasOwnProperty("type"))
                    switch (message.type) {
                    default:
                        return "type: enum value expected";
                    case 0:
                    case 1:
                    case 2:
                        break;
                    }
                return null;
            };

            /**
             * Creates a Core message from a plain object. Also converts values to their respective internal types.
             * @function fromObject
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {Object.<string,*>} object Plain object
             * @returns {pb.CPUInfo.Core} Core
             */
            Core.fromObject = function fromObject(object) {
                if (object instanceof $root.pb.CPUInfo.Core)
                    return object;
                var message = new $root.pb.CPUInfo.Core();
                if (object.index != null)
                    message.index = object.index >>> 0;
                if (object.usage != null)
                    message.usage = Number(object.usage);
                switch (object.type) {
                default:
                    if (typeof object.type === "number") {
                        message.type = object.type;
                        break;
                    }
                    break;
                case "PERFORMANCE":
                case 0:
                    message.type = 0;
                    break;
                case "EFFICIENCY":
                case 1:
                    message.type = 1;
                    break;
                case "HYPERTHREAD":
                case 2:
                    message.type = 2;
                    break;
                }
                return message;
            };

            /**
             * Creates a plain object from a Core message. Also converts values to other types if specified.
             * @function toObject
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {pb.CPUInfo.Core} message Core
             * @param {$protobuf.IConversionOptions} [options] Conversion options
             * @returns {Object.<string,*>} Plain object
             */
            Core.toObject = function toObject(message, options) {
                if (!options)
                    options = {};
                var object = {};
                if (options.defaults) {
                    object.index = 0;
                    object.usage = 0;
                    object.type = options.enums === String ? "PERFORMANCE" : 0;
                }
                if (message.index != null && message.hasOwnProperty("index"))
                    object.index = message.index;
                if (message.usage != null && message.hasOwnProperty("usage"))
                    object.usage = options.json && !isFinite(message.usage) ? String(message.usage) : message.usage;
                if (message.type != null && message.hasOwnProperty("type"))
                    object.type = options.enums === String ? $root.pb.CPUInfo.Core.Type[message.type] === undefined ? message.type : $root.pb.CPUInfo.Core.Type[message.type] : message.type;
                return object;
            };

            /**
             * Converts this Core to JSON.
             * @function toJSON
             * @memberof pb.CPUInfo.Core
             * @instance
             * @returns {Object.<string,*>} JSON object
             */
            Core.prototype.toJSON = function toJSON() {
                return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
            };

            /**
             * Gets the default type url for Core
             * @function getTypeUrl
             * @memberof pb.CPUInfo.Core
             * @static
             * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
             * @returns {string} The default type url
             */
            Core.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
                if (typeUrlPrefix === undefined) {
                    typeUrlPrefix = "type.googleapis.com";
                }
                return typeUrlPrefix + "/pb.CPUInfo.Core";
            };

            /**
             * Type enum.
             * @name pb.CPUInfo.Core.Type
             * @enum {number}
             * @property {number} PERFORMANCE=0 PERFORMANCE value
             * @property {number} EFFICIENCY=1 EFFICIENCY value
             * @property {number} HYPERTHREAD=2 HYPERTHREAD value
             */
            Core.Type = (function() {
                var valuesById = {}, values = Object.create(valuesById);
                values[valuesById[0] = "PERFORMANCE"] = 0;
                values[valuesById[1] = "EFFICIENCY"] = 1;
                values[valuesById[2] = "HYPERTHREAD"] = 2;
                return values;
            })();

            return Core;
        })();

        return CPUInfo;
    })();

    pb.MemoryInfo = (function() {

        /**
         * Properties of a MemoryInfo.
         * @memberof pb
         * @interface IMemoryInfo
         * @property {number|Long|null} [total] MemoryInfo total
         * @property {number|Long|null} [used] MemoryInfo used
         * @property {number|null} [percent] MemoryInfo percent
         * @property {number|Long|null} [cached] MemoryInfo cached
         * @property {number|Long|null} [buffers] MemoryInfo buffers
         * @property {number|Long|null} [shared] MemoryInfo shared
         * @property {number|Long|null} [zramUsed] MemoryInfo zramUsed
         * @property {number|Long|null} [zramTotal] MemoryInfo zramTotal
         * @property {number|Long|null} [swapTotal] MemoryInfo swapTotal
         * @property {number|Long|null} [swapUsed] MemoryInfo swapUsed
         */

        /**
         * Constructs a new MemoryInfo.
         * @memberof pb
         * @classdesc Represents a MemoryInfo.
         * @implements IMemoryInfo
         * @constructor
         * @param {pb.IMemoryInfo=} [properties] Properties to set
         */
        function MemoryInfo(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * MemoryInfo total.
         * @member {number|Long} total
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.total = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo used.
         * @member {number|Long} used
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.used = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo percent.
         * @member {number} percent
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.percent = 0;

        /**
         * MemoryInfo cached.
         * @member {number|Long} cached
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.cached = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo buffers.
         * @member {number|Long} buffers
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.buffers = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo shared.
         * @member {number|Long} shared
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.shared = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo zramUsed.
         * @member {number|Long} zramUsed
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.zramUsed = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo zramTotal.
         * @member {number|Long} zramTotal
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.zramTotal = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo swapTotal.
         * @member {number|Long} swapTotal
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.swapTotal = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * MemoryInfo swapUsed.
         * @member {number|Long} swapUsed
         * @memberof pb.MemoryInfo
         * @instance
         */
        MemoryInfo.prototype.swapUsed = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new MemoryInfo instance using the specified properties.
         * @function create
         * @memberof pb.MemoryInfo
         * @static
         * @param {pb.IMemoryInfo=} [properties] Properties to set
         * @returns {pb.MemoryInfo} MemoryInfo instance
         */
        MemoryInfo.create = function create(properties) {
            return new MemoryInfo(properties);
        };

        /**
         * Encodes the specified MemoryInfo message. Does not implicitly {@link pb.MemoryInfo.verify|verify} messages.
         * @function encode
         * @memberof pb.MemoryInfo
         * @static
         * @param {pb.IMemoryInfo} message MemoryInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MemoryInfo.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.total != null && Object.hasOwnProperty.call(message, "total"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.total);
            if (message.used != null && Object.hasOwnProperty.call(message, "used"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.used);
            if (message.percent != null && Object.hasOwnProperty.call(message, "percent"))
                writer.uint32(/* id 3, wireType 5 =*/29).float(message.percent);
            if (message.cached != null && Object.hasOwnProperty.call(message, "cached"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint64(message.cached);
            if (message.buffers != null && Object.hasOwnProperty.call(message, "buffers"))
                writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.buffers);
            if (message.shared != null && Object.hasOwnProperty.call(message, "shared"))
                writer.uint32(/* id 6, wireType 0 =*/48).uint64(message.shared);
            if (message.zramUsed != null && Object.hasOwnProperty.call(message, "zramUsed"))
                writer.uint32(/* id 7, wireType 0 =*/56).uint64(message.zramUsed);
            if (message.zramTotal != null && Object.hasOwnProperty.call(message, "zramTotal"))
                writer.uint32(/* id 8, wireType 0 =*/64).uint64(message.zramTotal);
            if (message.swapTotal != null && Object.hasOwnProperty.call(message, "swapTotal"))
                writer.uint32(/* id 9, wireType 0 =*/72).uint64(message.swapTotal);
            if (message.swapUsed != null && Object.hasOwnProperty.call(message, "swapUsed"))
                writer.uint32(/* id 10, wireType 0 =*/80).uint64(message.swapUsed);
            return writer;
        };

        /**
         * Encodes the specified MemoryInfo message, length delimited. Does not implicitly {@link pb.MemoryInfo.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.MemoryInfo
         * @static
         * @param {pb.IMemoryInfo} message MemoryInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        MemoryInfo.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a MemoryInfo message from the specified reader or buffer.
         * @function decode
         * @memberof pb.MemoryInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.MemoryInfo} MemoryInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MemoryInfo.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.MemoryInfo();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.total = reader.uint64();
                        break;
                    }
                case 2: {
                        message.used = reader.uint64();
                        break;
                    }
                case 3: {
                        message.percent = reader.float();
                        break;
                    }
                case 4: {
                        message.cached = reader.uint64();
                        break;
                    }
                case 5: {
                        message.buffers = reader.uint64();
                        break;
                    }
                case 6: {
                        message.shared = reader.uint64();
                        break;
                    }
                case 7: {
                        message.zramUsed = reader.uint64();
                        break;
                    }
                case 8: {
                        message.zramTotal = reader.uint64();
                        break;
                    }
                case 9: {
                        message.swapTotal = reader.uint64();
                        break;
                    }
                case 10: {
                        message.swapUsed = reader.uint64();
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
         * Decodes a MemoryInfo message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.MemoryInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.MemoryInfo} MemoryInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        MemoryInfo.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a MemoryInfo message.
         * @function verify
         * @memberof pb.MemoryInfo
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        MemoryInfo.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.total != null && message.hasOwnProperty("total"))
                if (!$util.isInteger(message.total) && !(message.total && $util.isInteger(message.total.low) && $util.isInteger(message.total.high)))
                    return "total: integer|Long expected";
            if (message.used != null && message.hasOwnProperty("used"))
                if (!$util.isInteger(message.used) && !(message.used && $util.isInteger(message.used.low) && $util.isInteger(message.used.high)))
                    return "used: integer|Long expected";
            if (message.percent != null && message.hasOwnProperty("percent"))
                if (typeof message.percent !== "number")
                    return "percent: number expected";
            if (message.cached != null && message.hasOwnProperty("cached"))
                if (!$util.isInteger(message.cached) && !(message.cached && $util.isInteger(message.cached.low) && $util.isInteger(message.cached.high)))
                    return "cached: integer|Long expected";
            if (message.buffers != null && message.hasOwnProperty("buffers"))
                if (!$util.isInteger(message.buffers) && !(message.buffers && $util.isInteger(message.buffers.low) && $util.isInteger(message.buffers.high)))
                    return "buffers: integer|Long expected";
            if (message.shared != null && message.hasOwnProperty("shared"))
                if (!$util.isInteger(message.shared) && !(message.shared && $util.isInteger(message.shared.low) && $util.isInteger(message.shared.high)))
                    return "shared: integer|Long expected";
            if (message.zramUsed != null && message.hasOwnProperty("zramUsed"))
                if (!$util.isInteger(message.zramUsed) && !(message.zramUsed && $util.isInteger(message.zramUsed.low) && $util.isInteger(message.zramUsed.high)))
                    return "zramUsed: integer|Long expected";
            if (message.zramTotal != null && message.hasOwnProperty("zramTotal"))
                if (!$util.isInteger(message.zramTotal) && !(message.zramTotal && $util.isInteger(message.zramTotal.low) && $util.isInteger(message.zramTotal.high)))
                    return "zramTotal: integer|Long expected";
            if (message.swapTotal != null && message.hasOwnProperty("swapTotal"))
                if (!$util.isInteger(message.swapTotal) && !(message.swapTotal && $util.isInteger(message.swapTotal.low) && $util.isInteger(message.swapTotal.high)))
                    return "swapTotal: integer|Long expected";
            if (message.swapUsed != null && message.hasOwnProperty("swapUsed"))
                if (!$util.isInteger(message.swapUsed) && !(message.swapUsed && $util.isInteger(message.swapUsed.low) && $util.isInteger(message.swapUsed.high)))
                    return "swapUsed: integer|Long expected";
            return null;
        };

        /**
         * Creates a MemoryInfo message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.MemoryInfo
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.MemoryInfo} MemoryInfo
         */
        MemoryInfo.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.MemoryInfo)
                return object;
            var message = new $root.pb.MemoryInfo();
            if (object.total != null)
                if ($util.Long)
                    (message.total = $util.Long.fromValue(object.total)).unsigned = true;
                else if (typeof object.total === "string")
                    message.total = parseInt(object.total, 10);
                else if (typeof object.total === "number")
                    message.total = object.total;
                else if (typeof object.total === "object")
                    message.total = new $util.LongBits(object.total.low >>> 0, object.total.high >>> 0).toNumber(true);
            if (object.used != null)
                if ($util.Long)
                    (message.used = $util.Long.fromValue(object.used)).unsigned = true;
                else if (typeof object.used === "string")
                    message.used = parseInt(object.used, 10);
                else if (typeof object.used === "number")
                    message.used = object.used;
                else if (typeof object.used === "object")
                    message.used = new $util.LongBits(object.used.low >>> 0, object.used.high >>> 0).toNumber(true);
            if (object.percent != null)
                message.percent = Number(object.percent);
            if (object.cached != null)
                if ($util.Long)
                    (message.cached = $util.Long.fromValue(object.cached)).unsigned = true;
                else if (typeof object.cached === "string")
                    message.cached = parseInt(object.cached, 10);
                else if (typeof object.cached === "number")
                    message.cached = object.cached;
                else if (typeof object.cached === "object")
                    message.cached = new $util.LongBits(object.cached.low >>> 0, object.cached.high >>> 0).toNumber(true);
            if (object.buffers != null)
                if ($util.Long)
                    (message.buffers = $util.Long.fromValue(object.buffers)).unsigned = true;
                else if (typeof object.buffers === "string")
                    message.buffers = parseInt(object.buffers, 10);
                else if (typeof object.buffers === "number")
                    message.buffers = object.buffers;
                else if (typeof object.buffers === "object")
                    message.buffers = new $util.LongBits(object.buffers.low >>> 0, object.buffers.high >>> 0).toNumber(true);
            if (object.shared != null)
                if ($util.Long)
                    (message.shared = $util.Long.fromValue(object.shared)).unsigned = true;
                else if (typeof object.shared === "string")
                    message.shared = parseInt(object.shared, 10);
                else if (typeof object.shared === "number")
                    message.shared = object.shared;
                else if (typeof object.shared === "object")
                    message.shared = new $util.LongBits(object.shared.low >>> 0, object.shared.high >>> 0).toNumber(true);
            if (object.zramUsed != null)
                if ($util.Long)
                    (message.zramUsed = $util.Long.fromValue(object.zramUsed)).unsigned = true;
                else if (typeof object.zramUsed === "string")
                    message.zramUsed = parseInt(object.zramUsed, 10);
                else if (typeof object.zramUsed === "number")
                    message.zramUsed = object.zramUsed;
                else if (typeof object.zramUsed === "object")
                    message.zramUsed = new $util.LongBits(object.zramUsed.low >>> 0, object.zramUsed.high >>> 0).toNumber(true);
            if (object.zramTotal != null)
                if ($util.Long)
                    (message.zramTotal = $util.Long.fromValue(object.zramTotal)).unsigned = true;
                else if (typeof object.zramTotal === "string")
                    message.zramTotal = parseInt(object.zramTotal, 10);
                else if (typeof object.zramTotal === "number")
                    message.zramTotal = object.zramTotal;
                else if (typeof object.zramTotal === "object")
                    message.zramTotal = new $util.LongBits(object.zramTotal.low >>> 0, object.zramTotal.high >>> 0).toNumber(true);
            if (object.swapTotal != null)
                if ($util.Long)
                    (message.swapTotal = $util.Long.fromValue(object.swapTotal)).unsigned = true;
                else if (typeof object.swapTotal === "string")
                    message.swapTotal = parseInt(object.swapTotal, 10);
                else if (typeof object.swapTotal === "number")
                    message.swapTotal = object.swapTotal;
                else if (typeof object.swapTotal === "object")
                    message.swapTotal = new $util.LongBits(object.swapTotal.low >>> 0, object.swapTotal.high >>> 0).toNumber(true);
            if (object.swapUsed != null)
                if ($util.Long)
                    (message.swapUsed = $util.Long.fromValue(object.swapUsed)).unsigned = true;
                else if (typeof object.swapUsed === "string")
                    message.swapUsed = parseInt(object.swapUsed, 10);
                else if (typeof object.swapUsed === "number")
                    message.swapUsed = object.swapUsed;
                else if (typeof object.swapUsed === "object")
                    message.swapUsed = new $util.LongBits(object.swapUsed.low >>> 0, object.swapUsed.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a MemoryInfo message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.MemoryInfo
         * @static
         * @param {pb.MemoryInfo} message MemoryInfo
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        MemoryInfo.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.total = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.total = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.used = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.used = options.longs === String ? "0" : 0;
                object.percent = 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.cached = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.cached = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.buffers = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.buffers = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.shared = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.shared = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.zramUsed = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.zramUsed = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.zramTotal = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.zramTotal = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.swapTotal = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.swapTotal = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.swapUsed = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.swapUsed = options.longs === String ? "0" : 0;
            }
            if (message.total != null && message.hasOwnProperty("total"))
                if (typeof message.total === "number")
                    object.total = options.longs === String ? String(message.total) : message.total;
                else
                    object.total = options.longs === String ? $util.Long.prototype.toString.call(message.total) : options.longs === Number ? new $util.LongBits(message.total.low >>> 0, message.total.high >>> 0).toNumber(true) : message.total;
            if (message.used != null && message.hasOwnProperty("used"))
                if (typeof message.used === "number")
                    object.used = options.longs === String ? String(message.used) : message.used;
                else
                    object.used = options.longs === String ? $util.Long.prototype.toString.call(message.used) : options.longs === Number ? new $util.LongBits(message.used.low >>> 0, message.used.high >>> 0).toNumber(true) : message.used;
            if (message.percent != null && message.hasOwnProperty("percent"))
                object.percent = options.json && !isFinite(message.percent) ? String(message.percent) : message.percent;
            if (message.cached != null && message.hasOwnProperty("cached"))
                if (typeof message.cached === "number")
                    object.cached = options.longs === String ? String(message.cached) : message.cached;
                else
                    object.cached = options.longs === String ? $util.Long.prototype.toString.call(message.cached) : options.longs === Number ? new $util.LongBits(message.cached.low >>> 0, message.cached.high >>> 0).toNumber(true) : message.cached;
            if (message.buffers != null && message.hasOwnProperty("buffers"))
                if (typeof message.buffers === "number")
                    object.buffers = options.longs === String ? String(message.buffers) : message.buffers;
                else
                    object.buffers = options.longs === String ? $util.Long.prototype.toString.call(message.buffers) : options.longs === Number ? new $util.LongBits(message.buffers.low >>> 0, message.buffers.high >>> 0).toNumber(true) : message.buffers;
            if (message.shared != null && message.hasOwnProperty("shared"))
                if (typeof message.shared === "number")
                    object.shared = options.longs === String ? String(message.shared) : message.shared;
                else
                    object.shared = options.longs === String ? $util.Long.prototype.toString.call(message.shared) : options.longs === Number ? new $util.LongBits(message.shared.low >>> 0, message.shared.high >>> 0).toNumber(true) : message.shared;
            if (message.zramUsed != null && message.hasOwnProperty("zramUsed"))
                if (typeof message.zramUsed === "number")
                    object.zramUsed = options.longs === String ? String(message.zramUsed) : message.zramUsed;
                else
                    object.zramUsed = options.longs === String ? $util.Long.prototype.toString.call(message.zramUsed) : options.longs === Number ? new $util.LongBits(message.zramUsed.low >>> 0, message.zramUsed.high >>> 0).toNumber(true) : message.zramUsed;
            if (message.zramTotal != null && message.hasOwnProperty("zramTotal"))
                if (typeof message.zramTotal === "number")
                    object.zramTotal = options.longs === String ? String(message.zramTotal) : message.zramTotal;
                else
                    object.zramTotal = options.longs === String ? $util.Long.prototype.toString.call(message.zramTotal) : options.longs === Number ? new $util.LongBits(message.zramTotal.low >>> 0, message.zramTotal.high >>> 0).toNumber(true) : message.zramTotal;
            if (message.swapTotal != null && message.hasOwnProperty("swapTotal"))
                if (typeof message.swapTotal === "number")
                    object.swapTotal = options.longs === String ? String(message.swapTotal) : message.swapTotal;
                else
                    object.swapTotal = options.longs === String ? $util.Long.prototype.toString.call(message.swapTotal) : options.longs === Number ? new $util.LongBits(message.swapTotal.low >>> 0, message.swapTotal.high >>> 0).toNumber(true) : message.swapTotal;
            if (message.swapUsed != null && message.hasOwnProperty("swapUsed"))
                if (typeof message.swapUsed === "number")
                    object.swapUsed = options.longs === String ? String(message.swapUsed) : message.swapUsed;
                else
                    object.swapUsed = options.longs === String ? $util.Long.prototype.toString.call(message.swapUsed) : options.longs === Number ? new $util.LongBits(message.swapUsed.low >>> 0, message.swapUsed.high >>> 0).toNumber(true) : message.swapUsed;
            return object;
        };

        /**
         * Converts this MemoryInfo to JSON.
         * @function toJSON
         * @memberof pb.MemoryInfo
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        MemoryInfo.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for MemoryInfo
         * @function getTypeUrl
         * @memberof pb.MemoryInfo
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        MemoryInfo.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.MemoryInfo";
        };

        return MemoryInfo;
    })();

    pb.Hook = (function() {

        /**
         * Properties of a Hook.
         * @memberof pb
         * @interface IHook
         * @property {string|null} [id] Hook id
         * @property {string|null} [name] Hook name
         * @property {string|null} [description] Hook description
         * @property {boolean|null} [installed] Hook installed
         * @property {string|null} [targetCmd] Hook targetCmd
         */

        /**
         * Constructs a new Hook.
         * @memberof pb
         * @classdesc Represents a Hook.
         * @implements IHook
         * @constructor
         * @param {pb.IHook=} [properties] Properties to set
         */
        function Hook(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Hook id.
         * @member {string} id
         * @memberof pb.Hook
         * @instance
         */
        Hook.prototype.id = "";

        /**
         * Hook name.
         * @member {string} name
         * @memberof pb.Hook
         * @instance
         */
        Hook.prototype.name = "";

        /**
         * Hook description.
         * @member {string} description
         * @memberof pb.Hook
         * @instance
         */
        Hook.prototype.description = "";

        /**
         * Hook installed.
         * @member {boolean} installed
         * @memberof pb.Hook
         * @instance
         */
        Hook.prototype.installed = false;

        /**
         * Hook targetCmd.
         * @member {string} targetCmd
         * @memberof pb.Hook
         * @instance
         */
        Hook.prototype.targetCmd = "";

        /**
         * Creates a new Hook instance using the specified properties.
         * @function create
         * @memberof pb.Hook
         * @static
         * @param {pb.IHook=} [properties] Properties to set
         * @returns {pb.Hook} Hook instance
         */
        Hook.create = function create(properties) {
            return new Hook(properties);
        };

        /**
         * Encodes the specified Hook message. Does not implicitly {@link pb.Hook.verify|verify} messages.
         * @function encode
         * @memberof pb.Hook
         * @static
         * @param {pb.IHook} message Hook message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Hook.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.id != null && Object.hasOwnProperty.call(message, "id"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.id);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.description != null && Object.hasOwnProperty.call(message, "description"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.description);
            if (message.installed != null && Object.hasOwnProperty.call(message, "installed"))
                writer.uint32(/* id 4, wireType 0 =*/32).bool(message.installed);
            if (message.targetCmd != null && Object.hasOwnProperty.call(message, "targetCmd"))
                writer.uint32(/* id 5, wireType 2 =*/42).string(message.targetCmd);
            return writer;
        };

        /**
         * Encodes the specified Hook message, length delimited. Does not implicitly {@link pb.Hook.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.Hook
         * @static
         * @param {pb.IHook} message Hook message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Hook.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Hook message from the specified reader or buffer.
         * @function decode
         * @memberof pb.Hook
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.Hook} Hook
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Hook.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.Hook();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.id = reader.string();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.description = reader.string();
                        break;
                    }
                case 4: {
                        message.installed = reader.bool();
                        break;
                    }
                case 5: {
                        message.targetCmd = reader.string();
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
         * Decodes a Hook message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.Hook
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.Hook} Hook
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Hook.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Hook message.
         * @function verify
         * @memberof pb.Hook
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Hook.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.id != null && message.hasOwnProperty("id"))
                if (!$util.isString(message.id))
                    return "id: string expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.description != null && message.hasOwnProperty("description"))
                if (!$util.isString(message.description))
                    return "description: string expected";
            if (message.installed != null && message.hasOwnProperty("installed"))
                if (typeof message.installed !== "boolean")
                    return "installed: boolean expected";
            if (message.targetCmd != null && message.hasOwnProperty("targetCmd"))
                if (!$util.isString(message.targetCmd))
                    return "targetCmd: string expected";
            return null;
        };

        /**
         * Creates a Hook message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.Hook
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.Hook} Hook
         */
        Hook.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.Hook)
                return object;
            var message = new $root.pb.Hook();
            if (object.id != null)
                message.id = String(object.id);
            if (object.name != null)
                message.name = String(object.name);
            if (object.description != null)
                message.description = String(object.description);
            if (object.installed != null)
                message.installed = Boolean(object.installed);
            if (object.targetCmd != null)
                message.targetCmd = String(object.targetCmd);
            return message;
        };

        /**
         * Creates a plain object from a Hook message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.Hook
         * @static
         * @param {pb.Hook} message Hook
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Hook.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.id = "";
                object.name = "";
                object.description = "";
                object.installed = false;
                object.targetCmd = "";
            }
            if (message.id != null && message.hasOwnProperty("id"))
                object.id = message.id;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.description != null && message.hasOwnProperty("description"))
                object.description = message.description;
            if (message.installed != null && message.hasOwnProperty("installed"))
                object.installed = message.installed;
            if (message.targetCmd != null && message.hasOwnProperty("targetCmd"))
                object.targetCmd = message.targetCmd;
            return object;
        };

        /**
         * Converts this Hook to JSON.
         * @function toJSON
         * @memberof pb.Hook
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Hook.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Hook
         * @function getTypeUrl
         * @memberof pb.Hook
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Hook.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.Hook";
        };

        return Hook;
    })();

    pb.HookRequest = (function() {

        /**
         * Properties of a HookRequest.
         * @memberof pb
         * @interface IHookRequest
         * @property {string|null} [id] HookRequest id
         * @property {boolean|null} [install] HookRequest install
         */

        /**
         * Constructs a new HookRequest.
         * @memberof pb
         * @classdesc Represents a HookRequest.
         * @implements IHookRequest
         * @constructor
         * @param {pb.IHookRequest=} [properties] Properties to set
         */
        function HookRequest(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * HookRequest id.
         * @member {string} id
         * @memberof pb.HookRequest
         * @instance
         */
        HookRequest.prototype.id = "";

        /**
         * HookRequest install.
         * @member {boolean} install
         * @memberof pb.HookRequest
         * @instance
         */
        HookRequest.prototype.install = false;

        /**
         * Creates a new HookRequest instance using the specified properties.
         * @function create
         * @memberof pb.HookRequest
         * @static
         * @param {pb.IHookRequest=} [properties] Properties to set
         * @returns {pb.HookRequest} HookRequest instance
         */
        HookRequest.create = function create(properties) {
            return new HookRequest(properties);
        };

        /**
         * Encodes the specified HookRequest message. Does not implicitly {@link pb.HookRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.HookRequest
         * @static
         * @param {pb.IHookRequest} message HookRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        HookRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.id != null && Object.hasOwnProperty.call(message, "id"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.id);
            if (message.install != null && Object.hasOwnProperty.call(message, "install"))
                writer.uint32(/* id 2, wireType 0 =*/16).bool(message.install);
            return writer;
        };

        /**
         * Encodes the specified HookRequest message, length delimited. Does not implicitly {@link pb.HookRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.HookRequest
         * @static
         * @param {pb.IHookRequest} message HookRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        HookRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a HookRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.HookRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.HookRequest} HookRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        HookRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.HookRequest();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.id = reader.string();
                        break;
                    }
                case 2: {
                        message.install = reader.bool();
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
         * Decodes a HookRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.HookRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.HookRequest} HookRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        HookRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a HookRequest message.
         * @function verify
         * @memberof pb.HookRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        HookRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.id != null && message.hasOwnProperty("id"))
                if (!$util.isString(message.id))
                    return "id: string expected";
            if (message.install != null && message.hasOwnProperty("install"))
                if (typeof message.install !== "boolean")
                    return "install: boolean expected";
            return null;
        };

        /**
         * Creates a HookRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.HookRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.HookRequest} HookRequest
         */
        HookRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.HookRequest)
                return object;
            var message = new $root.pb.HookRequest();
            if (object.id != null)
                message.id = String(object.id);
            if (object.install != null)
                message.install = Boolean(object.install);
            return message;
        };

        /**
         * Creates a plain object from a HookRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.HookRequest
         * @static
         * @param {pb.HookRequest} message HookRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        HookRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.id = "";
                object.install = false;
            }
            if (message.id != null && message.hasOwnProperty("id"))
                object.id = message.id;
            if (message.install != null && message.hasOwnProperty("install"))
                object.install = message.install;
            return object;
        };

        /**
         * Converts this HookRequest to JSON.
         * @function toJSON
         * @memberof pb.HookRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        HookRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for HookRequest
         * @function getTypeUrl
         * @memberof pb.HookRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        HookRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.HookRequest";
        };

        return HookRequest;
    })();

    pb.HookResponse = (function() {

        /**
         * Properties of a HookResponse.
         * @memberof pb
         * @interface IHookResponse
         * @property {boolean|null} [success] HookResponse success
         * @property {string|null} [message] HookResponse message
         */

        /**
         * Constructs a new HookResponse.
         * @memberof pb
         * @classdesc Represents a HookResponse.
         * @implements IHookResponse
         * @constructor
         * @param {pb.IHookResponse=} [properties] Properties to set
         */
        function HookResponse(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * HookResponse success.
         * @member {boolean} success
         * @memberof pb.HookResponse
         * @instance
         */
        HookResponse.prototype.success = false;

        /**
         * HookResponse message.
         * @member {string} message
         * @memberof pb.HookResponse
         * @instance
         */
        HookResponse.prototype.message = "";

        /**
         * Creates a new HookResponse instance using the specified properties.
         * @function create
         * @memberof pb.HookResponse
         * @static
         * @param {pb.IHookResponse=} [properties] Properties to set
         * @returns {pb.HookResponse} HookResponse instance
         */
        HookResponse.create = function create(properties) {
            return new HookResponse(properties);
        };

        /**
         * Encodes the specified HookResponse message. Does not implicitly {@link pb.HookResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.HookResponse
         * @static
         * @param {pb.IHookResponse} message HookResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        HookResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified HookResponse message, length delimited. Does not implicitly {@link pb.HookResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.HookResponse
         * @static
         * @param {pb.IHookResponse} message HookResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        HookResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a HookResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.HookResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.HookResponse} HookResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        HookResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.HookResponse();
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
         * Decodes a HookResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.HookResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.HookResponse} HookResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        HookResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a HookResponse message.
         * @function verify
         * @memberof pb.HookResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        HookResponse.verify = function verify(message) {
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
         * Creates a HookResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.HookResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.HookResponse} HookResponse
         */
        HookResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.HookResponse)
                return object;
            var message = new $root.pb.HookResponse();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from a HookResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.HookResponse
         * @static
         * @param {pb.HookResponse} message HookResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        HookResponse.toObject = function toObject(message, options) {
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
         * Converts this HookResponse to JSON.
         * @function toJSON
         * @memberof pb.HookResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        HookResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for HookResponse
         * @function getTypeUrl
         * @memberof pb.HookResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        HookResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.HookResponse";
        };

        return HookResponse;
    })();

    pb.NetworkInterface = (function() {

        /**
         * Properties of a NetworkInterface.
         * @memberof pb
         * @interface INetworkInterface
         * @property {string|null} [name] NetworkInterface name
         * @property {number|Long|null} [recvBytes] NetworkInterface recvBytes
         * @property {number|Long|null} [sentBytes] NetworkInterface sentBytes
         */

        /**
         * Constructs a new NetworkInterface.
         * @memberof pb
         * @classdesc Represents a NetworkInterface.
         * @implements INetworkInterface
         * @constructor
         * @param {pb.INetworkInterface=} [properties] Properties to set
         */
        function NetworkInterface(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * NetworkInterface name.
         * @member {string} name
         * @memberof pb.NetworkInterface
         * @instance
         */
        NetworkInterface.prototype.name = "";

        /**
         * NetworkInterface recvBytes.
         * @member {number|Long} recvBytes
         * @memberof pb.NetworkInterface
         * @instance
         */
        NetworkInterface.prototype.recvBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * NetworkInterface sentBytes.
         * @member {number|Long} sentBytes
         * @memberof pb.NetworkInterface
         * @instance
         */
        NetworkInterface.prototype.sentBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new NetworkInterface instance using the specified properties.
         * @function create
         * @memberof pb.NetworkInterface
         * @static
         * @param {pb.INetworkInterface=} [properties] Properties to set
         * @returns {pb.NetworkInterface} NetworkInterface instance
         */
        NetworkInterface.create = function create(properties) {
            return new NetworkInterface(properties);
        };

        /**
         * Encodes the specified NetworkInterface message. Does not implicitly {@link pb.NetworkInterface.verify|verify} messages.
         * @function encode
         * @memberof pb.NetworkInterface
         * @static
         * @param {pb.INetworkInterface} message NetworkInterface message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        NetworkInterface.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.name);
            if (message.recvBytes != null && Object.hasOwnProperty.call(message, "recvBytes"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.recvBytes);
            if (message.sentBytes != null && Object.hasOwnProperty.call(message, "sentBytes"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.sentBytes);
            return writer;
        };

        /**
         * Encodes the specified NetworkInterface message, length delimited. Does not implicitly {@link pb.NetworkInterface.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.NetworkInterface
         * @static
         * @param {pb.INetworkInterface} message NetworkInterface message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        NetworkInterface.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a NetworkInterface message from the specified reader or buffer.
         * @function decode
         * @memberof pb.NetworkInterface
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.NetworkInterface} NetworkInterface
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        NetworkInterface.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.NetworkInterface();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.name = reader.string();
                        break;
                    }
                case 2: {
                        message.recvBytes = reader.uint64();
                        break;
                    }
                case 3: {
                        message.sentBytes = reader.uint64();
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
         * Decodes a NetworkInterface message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.NetworkInterface
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.NetworkInterface} NetworkInterface
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        NetworkInterface.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a NetworkInterface message.
         * @function verify
         * @memberof pb.NetworkInterface
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        NetworkInterface.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.recvBytes != null && message.hasOwnProperty("recvBytes"))
                if (!$util.isInteger(message.recvBytes) && !(message.recvBytes && $util.isInteger(message.recvBytes.low) && $util.isInteger(message.recvBytes.high)))
                    return "recvBytes: integer|Long expected";
            if (message.sentBytes != null && message.hasOwnProperty("sentBytes"))
                if (!$util.isInteger(message.sentBytes) && !(message.sentBytes && $util.isInteger(message.sentBytes.low) && $util.isInteger(message.sentBytes.high)))
                    return "sentBytes: integer|Long expected";
            return null;
        };

        /**
         * Creates a NetworkInterface message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.NetworkInterface
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.NetworkInterface} NetworkInterface
         */
        NetworkInterface.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.NetworkInterface)
                return object;
            var message = new $root.pb.NetworkInterface();
            if (object.name != null)
                message.name = String(object.name);
            if (object.recvBytes != null)
                if ($util.Long)
                    (message.recvBytes = $util.Long.fromValue(object.recvBytes)).unsigned = true;
                else if (typeof object.recvBytes === "string")
                    message.recvBytes = parseInt(object.recvBytes, 10);
                else if (typeof object.recvBytes === "number")
                    message.recvBytes = object.recvBytes;
                else if (typeof object.recvBytes === "object")
                    message.recvBytes = new $util.LongBits(object.recvBytes.low >>> 0, object.recvBytes.high >>> 0).toNumber(true);
            if (object.sentBytes != null)
                if ($util.Long)
                    (message.sentBytes = $util.Long.fromValue(object.sentBytes)).unsigned = true;
                else if (typeof object.sentBytes === "string")
                    message.sentBytes = parseInt(object.sentBytes, 10);
                else if (typeof object.sentBytes === "number")
                    message.sentBytes = object.sentBytes;
                else if (typeof object.sentBytes === "object")
                    message.sentBytes = new $util.LongBits(object.sentBytes.low >>> 0, object.sentBytes.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a NetworkInterface message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.NetworkInterface
         * @static
         * @param {pb.NetworkInterface} message NetworkInterface
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        NetworkInterface.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.name = "";
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.recvBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.recvBytes = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.sentBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.sentBytes = options.longs === String ? "0" : 0;
            }
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.recvBytes != null && message.hasOwnProperty("recvBytes"))
                if (typeof message.recvBytes === "number")
                    object.recvBytes = options.longs === String ? String(message.recvBytes) : message.recvBytes;
                else
                    object.recvBytes = options.longs === String ? $util.Long.prototype.toString.call(message.recvBytes) : options.longs === Number ? new $util.LongBits(message.recvBytes.low >>> 0, message.recvBytes.high >>> 0).toNumber(true) : message.recvBytes;
            if (message.sentBytes != null && message.hasOwnProperty("sentBytes"))
                if (typeof message.sentBytes === "number")
                    object.sentBytes = options.longs === String ? String(message.sentBytes) : message.sentBytes;
                else
                    object.sentBytes = options.longs === String ? $util.Long.prototype.toString.call(message.sentBytes) : options.longs === Number ? new $util.LongBits(message.sentBytes.low >>> 0, message.sentBytes.high >>> 0).toNumber(true) : message.sentBytes;
            return object;
        };

        /**
         * Converts this NetworkInterface to JSON.
         * @function toJSON
         * @memberof pb.NetworkInterface
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        NetworkInterface.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for NetworkInterface
         * @function getTypeUrl
         * @memberof pb.NetworkInterface
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        NetworkInterface.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.NetworkInterface";
        };

        return NetworkInterface;
    })();

    pb.DiskDevice = (function() {

        /**
         * Properties of a DiskDevice.
         * @memberof pb
         * @interface IDiskDevice
         * @property {string|null} [name] DiskDevice name
         * @property {number|Long|null} [readBytes] DiskDevice readBytes
         * @property {number|Long|null} [writeBytes] DiskDevice writeBytes
         */

        /**
         * Constructs a new DiskDevice.
         * @memberof pb
         * @classdesc Represents a DiskDevice.
         * @implements IDiskDevice
         * @constructor
         * @param {pb.IDiskDevice=} [properties] Properties to set
         */
        function DiskDevice(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * DiskDevice name.
         * @member {string} name
         * @memberof pb.DiskDevice
         * @instance
         */
        DiskDevice.prototype.name = "";

        /**
         * DiskDevice readBytes.
         * @member {number|Long} readBytes
         * @memberof pb.DiskDevice
         * @instance
         */
        DiskDevice.prototype.readBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * DiskDevice writeBytes.
         * @member {number|Long} writeBytes
         * @memberof pb.DiskDevice
         * @instance
         */
        DiskDevice.prototype.writeBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Creates a new DiskDevice instance using the specified properties.
         * @function create
         * @memberof pb.DiskDevice
         * @static
         * @param {pb.IDiskDevice=} [properties] Properties to set
         * @returns {pb.DiskDevice} DiskDevice instance
         */
        DiskDevice.create = function create(properties) {
            return new DiskDevice(properties);
        };

        /**
         * Encodes the specified DiskDevice message. Does not implicitly {@link pb.DiskDevice.verify|verify} messages.
         * @function encode
         * @memberof pb.DiskDevice
         * @static
         * @param {pb.IDiskDevice} message DiskDevice message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        DiskDevice.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 1, wireType 2 =*/10).string(message.name);
            if (message.readBytes != null && Object.hasOwnProperty.call(message, "readBytes"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.readBytes);
            if (message.writeBytes != null && Object.hasOwnProperty.call(message, "writeBytes"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.writeBytes);
            return writer;
        };

        /**
         * Encodes the specified DiskDevice message, length delimited. Does not implicitly {@link pb.DiskDevice.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.DiskDevice
         * @static
         * @param {pb.IDiskDevice} message DiskDevice message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        DiskDevice.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a DiskDevice message from the specified reader or buffer.
         * @function decode
         * @memberof pb.DiskDevice
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.DiskDevice} DiskDevice
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        DiskDevice.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.DiskDevice();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.name = reader.string();
                        break;
                    }
                case 2: {
                        message.readBytes = reader.uint64();
                        break;
                    }
                case 3: {
                        message.writeBytes = reader.uint64();
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
         * Decodes a DiskDevice message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.DiskDevice
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.DiskDevice} DiskDevice
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        DiskDevice.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a DiskDevice message.
         * @function verify
         * @memberof pb.DiskDevice
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        DiskDevice.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.readBytes != null && message.hasOwnProperty("readBytes"))
                if (!$util.isInteger(message.readBytes) && !(message.readBytes && $util.isInteger(message.readBytes.low) && $util.isInteger(message.readBytes.high)))
                    return "readBytes: integer|Long expected";
            if (message.writeBytes != null && message.hasOwnProperty("writeBytes"))
                if (!$util.isInteger(message.writeBytes) && !(message.writeBytes && $util.isInteger(message.writeBytes.low) && $util.isInteger(message.writeBytes.high)))
                    return "writeBytes: integer|Long expected";
            return null;
        };

        /**
         * Creates a DiskDevice message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.DiskDevice
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.DiskDevice} DiskDevice
         */
        DiskDevice.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.DiskDevice)
                return object;
            var message = new $root.pb.DiskDevice();
            if (object.name != null)
                message.name = String(object.name);
            if (object.readBytes != null)
                if ($util.Long)
                    (message.readBytes = $util.Long.fromValue(object.readBytes)).unsigned = true;
                else if (typeof object.readBytes === "string")
                    message.readBytes = parseInt(object.readBytes, 10);
                else if (typeof object.readBytes === "number")
                    message.readBytes = object.readBytes;
                else if (typeof object.readBytes === "object")
                    message.readBytes = new $util.LongBits(object.readBytes.low >>> 0, object.readBytes.high >>> 0).toNumber(true);
            if (object.writeBytes != null)
                if ($util.Long)
                    (message.writeBytes = $util.Long.fromValue(object.writeBytes)).unsigned = true;
                else if (typeof object.writeBytes === "string")
                    message.writeBytes = parseInt(object.writeBytes, 10);
                else if (typeof object.writeBytes === "number")
                    message.writeBytes = object.writeBytes;
                else if (typeof object.writeBytes === "object")
                    message.writeBytes = new $util.LongBits(object.writeBytes.low >>> 0, object.writeBytes.high >>> 0).toNumber(true);
            return message;
        };

        /**
         * Creates a plain object from a DiskDevice message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.DiskDevice
         * @static
         * @param {pb.DiskDevice} message DiskDevice
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        DiskDevice.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                object.name = "";
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.readBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.readBytes = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.writeBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.writeBytes = options.longs === String ? "0" : 0;
            }
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.readBytes != null && message.hasOwnProperty("readBytes"))
                if (typeof message.readBytes === "number")
                    object.readBytes = options.longs === String ? String(message.readBytes) : message.readBytes;
                else
                    object.readBytes = options.longs === String ? $util.Long.prototype.toString.call(message.readBytes) : options.longs === Number ? new $util.LongBits(message.readBytes.low >>> 0, message.readBytes.high >>> 0).toNumber(true) : message.readBytes;
            if (message.writeBytes != null && message.hasOwnProperty("writeBytes"))
                if (typeof message.writeBytes === "number")
                    object.writeBytes = options.longs === String ? String(message.writeBytes) : message.writeBytes;
                else
                    object.writeBytes = options.longs === String ? $util.Long.prototype.toString.call(message.writeBytes) : options.longs === Number ? new $util.LongBits(message.writeBytes.low >>> 0, message.writeBytes.high >>> 0).toNumber(true) : message.writeBytes;
            return object;
        };

        /**
         * Converts this DiskDevice to JSON.
         * @function toJSON
         * @memberof pb.DiskDevice
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        DiskDevice.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for DiskDevice
         * @function getTypeUrl
         * @memberof pb.DiskDevice
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        DiskDevice.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.DiskDevice";
        };

        return DiskDevice;
    })();

    pb.IOInfo = (function() {

        /**
         * Properties of a IOInfo.
         * @memberof pb
         * @interface IIOInfo
         * @property {number|Long|null} [totalReadBytes] IOInfo totalReadBytes
         * @property {number|Long|null} [totalWriteBytes] IOInfo totalWriteBytes
         * @property {number|Long|null} [totalNetRecvBytes] IOInfo totalNetRecvBytes
         * @property {number|Long|null} [totalNetSentBytes] IOInfo totalNetSentBytes
         * @property {Array.<pb.INetworkInterface>|null} [networks] IOInfo networks
         * @property {Array.<pb.IDiskDevice>|null} [disks] IOInfo disks
         */

        /**
         * Constructs a new IOInfo.
         * @memberof pb
         * @classdesc Represents a IOInfo.
         * @implements IIOInfo
         * @constructor
         * @param {pb.IIOInfo=} [properties] Properties to set
         */
        function IOInfo(properties) {
            this.networks = [];
            this.disks = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * IOInfo totalReadBytes.
         * @member {number|Long} totalReadBytes
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.totalReadBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * IOInfo totalWriteBytes.
         * @member {number|Long} totalWriteBytes
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.totalWriteBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * IOInfo totalNetRecvBytes.
         * @member {number|Long} totalNetRecvBytes
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.totalNetRecvBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * IOInfo totalNetSentBytes.
         * @member {number|Long} totalNetSentBytes
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.totalNetSentBytes = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * IOInfo networks.
         * @member {Array.<pb.INetworkInterface>} networks
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.networks = $util.emptyArray;

        /**
         * IOInfo disks.
         * @member {Array.<pb.IDiskDevice>} disks
         * @memberof pb.IOInfo
         * @instance
         */
        IOInfo.prototype.disks = $util.emptyArray;

        /**
         * Creates a new IOInfo instance using the specified properties.
         * @function create
         * @memberof pb.IOInfo
         * @static
         * @param {pb.IIOInfo=} [properties] Properties to set
         * @returns {pb.IOInfo} IOInfo instance
         */
        IOInfo.create = function create(properties) {
            return new IOInfo(properties);
        };

        /**
         * Encodes the specified IOInfo message. Does not implicitly {@link pb.IOInfo.verify|verify} messages.
         * @function encode
         * @memberof pb.IOInfo
         * @static
         * @param {pb.IIOInfo} message IOInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        IOInfo.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.totalReadBytes != null && Object.hasOwnProperty.call(message, "totalReadBytes"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.totalReadBytes);
            if (message.totalWriteBytes != null && Object.hasOwnProperty.call(message, "totalWriteBytes"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.totalWriteBytes);
            if (message.totalNetRecvBytes != null && Object.hasOwnProperty.call(message, "totalNetRecvBytes"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.totalNetRecvBytes);
            if (message.totalNetSentBytes != null && Object.hasOwnProperty.call(message, "totalNetSentBytes"))
                writer.uint32(/* id 4, wireType 0 =*/32).uint64(message.totalNetSentBytes);
            if (message.networks != null && message.networks.length)
                for (var i = 0; i < message.networks.length; ++i)
                    $root.pb.NetworkInterface.encode(message.networks[i], writer.uint32(/* id 5, wireType 2 =*/42).fork()).ldelim();
            if (message.disks != null && message.disks.length)
                for (var i = 0; i < message.disks.length; ++i)
                    $root.pb.DiskDevice.encode(message.disks[i], writer.uint32(/* id 6, wireType 2 =*/50).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified IOInfo message, length delimited. Does not implicitly {@link pb.IOInfo.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.IOInfo
         * @static
         * @param {pb.IIOInfo} message IOInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        IOInfo.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a IOInfo message from the specified reader or buffer.
         * @function decode
         * @memberof pb.IOInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.IOInfo} IOInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        IOInfo.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.IOInfo();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.totalReadBytes = reader.uint64();
                        break;
                    }
                case 2: {
                        message.totalWriteBytes = reader.uint64();
                        break;
                    }
                case 3: {
                        message.totalNetRecvBytes = reader.uint64();
                        break;
                    }
                case 4: {
                        message.totalNetSentBytes = reader.uint64();
                        break;
                    }
                case 5: {
                        if (!(message.networks && message.networks.length))
                            message.networks = [];
                        message.networks.push($root.pb.NetworkInterface.decode(reader, reader.uint32()));
                        break;
                    }
                case 6: {
                        if (!(message.disks && message.disks.length))
                            message.disks = [];
                        message.disks.push($root.pb.DiskDevice.decode(reader, reader.uint32()));
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
         * Decodes a IOInfo message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.IOInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.IOInfo} IOInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        IOInfo.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a IOInfo message.
         * @function verify
         * @memberof pb.IOInfo
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        IOInfo.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.totalReadBytes != null && message.hasOwnProperty("totalReadBytes"))
                if (!$util.isInteger(message.totalReadBytes) && !(message.totalReadBytes && $util.isInteger(message.totalReadBytes.low) && $util.isInteger(message.totalReadBytes.high)))
                    return "totalReadBytes: integer|Long expected";
            if (message.totalWriteBytes != null && message.hasOwnProperty("totalWriteBytes"))
                if (!$util.isInteger(message.totalWriteBytes) && !(message.totalWriteBytes && $util.isInteger(message.totalWriteBytes.low) && $util.isInteger(message.totalWriteBytes.high)))
                    return "totalWriteBytes: integer|Long expected";
            if (message.totalNetRecvBytes != null && message.hasOwnProperty("totalNetRecvBytes"))
                if (!$util.isInteger(message.totalNetRecvBytes) && !(message.totalNetRecvBytes && $util.isInteger(message.totalNetRecvBytes.low) && $util.isInteger(message.totalNetRecvBytes.high)))
                    return "totalNetRecvBytes: integer|Long expected";
            if (message.totalNetSentBytes != null && message.hasOwnProperty("totalNetSentBytes"))
                if (!$util.isInteger(message.totalNetSentBytes) && !(message.totalNetSentBytes && $util.isInteger(message.totalNetSentBytes.low) && $util.isInteger(message.totalNetSentBytes.high)))
                    return "totalNetSentBytes: integer|Long expected";
            if (message.networks != null && message.hasOwnProperty("networks")) {
                if (!Array.isArray(message.networks))
                    return "networks: array expected";
                for (var i = 0; i < message.networks.length; ++i) {
                    var error = $root.pb.NetworkInterface.verify(message.networks[i]);
                    if (error)
                        return "networks." + error;
                }
            }
            if (message.disks != null && message.hasOwnProperty("disks")) {
                if (!Array.isArray(message.disks))
                    return "disks: array expected";
                for (var i = 0; i < message.disks.length; ++i) {
                    var error = $root.pb.DiskDevice.verify(message.disks[i]);
                    if (error)
                        return "disks." + error;
                }
            }
            return null;
        };

        /**
         * Creates a IOInfo message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.IOInfo
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.IOInfo} IOInfo
         */
        IOInfo.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.IOInfo)
                return object;
            var message = new $root.pb.IOInfo();
            if (object.totalReadBytes != null)
                if ($util.Long)
                    (message.totalReadBytes = $util.Long.fromValue(object.totalReadBytes)).unsigned = true;
                else if (typeof object.totalReadBytes === "string")
                    message.totalReadBytes = parseInt(object.totalReadBytes, 10);
                else if (typeof object.totalReadBytes === "number")
                    message.totalReadBytes = object.totalReadBytes;
                else if (typeof object.totalReadBytes === "object")
                    message.totalReadBytes = new $util.LongBits(object.totalReadBytes.low >>> 0, object.totalReadBytes.high >>> 0).toNumber(true);
            if (object.totalWriteBytes != null)
                if ($util.Long)
                    (message.totalWriteBytes = $util.Long.fromValue(object.totalWriteBytes)).unsigned = true;
                else if (typeof object.totalWriteBytes === "string")
                    message.totalWriteBytes = parseInt(object.totalWriteBytes, 10);
                else if (typeof object.totalWriteBytes === "number")
                    message.totalWriteBytes = object.totalWriteBytes;
                else if (typeof object.totalWriteBytes === "object")
                    message.totalWriteBytes = new $util.LongBits(object.totalWriteBytes.low >>> 0, object.totalWriteBytes.high >>> 0).toNumber(true);
            if (object.totalNetRecvBytes != null)
                if ($util.Long)
                    (message.totalNetRecvBytes = $util.Long.fromValue(object.totalNetRecvBytes)).unsigned = true;
                else if (typeof object.totalNetRecvBytes === "string")
                    message.totalNetRecvBytes = parseInt(object.totalNetRecvBytes, 10);
                else if (typeof object.totalNetRecvBytes === "number")
                    message.totalNetRecvBytes = object.totalNetRecvBytes;
                else if (typeof object.totalNetRecvBytes === "object")
                    message.totalNetRecvBytes = new $util.LongBits(object.totalNetRecvBytes.low >>> 0, object.totalNetRecvBytes.high >>> 0).toNumber(true);
            if (object.totalNetSentBytes != null)
                if ($util.Long)
                    (message.totalNetSentBytes = $util.Long.fromValue(object.totalNetSentBytes)).unsigned = true;
                else if (typeof object.totalNetSentBytes === "string")
                    message.totalNetSentBytes = parseInt(object.totalNetSentBytes, 10);
                else if (typeof object.totalNetSentBytes === "number")
                    message.totalNetSentBytes = object.totalNetSentBytes;
                else if (typeof object.totalNetSentBytes === "object")
                    message.totalNetSentBytes = new $util.LongBits(object.totalNetSentBytes.low >>> 0, object.totalNetSentBytes.high >>> 0).toNumber(true);
            if (object.networks) {
                if (!Array.isArray(object.networks))
                    throw TypeError(".pb.IOInfo.networks: array expected");
                message.networks = [];
                for (var i = 0; i < object.networks.length; ++i) {
                    if (typeof object.networks[i] !== "object")
                        throw TypeError(".pb.IOInfo.networks: object expected");
                    message.networks[i] = $root.pb.NetworkInterface.fromObject(object.networks[i]);
                }
            }
            if (object.disks) {
                if (!Array.isArray(object.disks))
                    throw TypeError(".pb.IOInfo.disks: array expected");
                message.disks = [];
                for (var i = 0; i < object.disks.length; ++i) {
                    if (typeof object.disks[i] !== "object")
                        throw TypeError(".pb.IOInfo.disks: object expected");
                    message.disks[i] = $root.pb.DiskDevice.fromObject(object.disks[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a IOInfo message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.IOInfo
         * @static
         * @param {pb.IOInfo} message IOInfo
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        IOInfo.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults) {
                object.networks = [];
                object.disks = [];
            }
            if (options.defaults) {
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.totalReadBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.totalReadBytes = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.totalWriteBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.totalWriteBytes = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.totalNetRecvBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.totalNetRecvBytes = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.totalNetSentBytes = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.totalNetSentBytes = options.longs === String ? "0" : 0;
            }
            if (message.totalReadBytes != null && message.hasOwnProperty("totalReadBytes"))
                if (typeof message.totalReadBytes === "number")
                    object.totalReadBytes = options.longs === String ? String(message.totalReadBytes) : message.totalReadBytes;
                else
                    object.totalReadBytes = options.longs === String ? $util.Long.prototype.toString.call(message.totalReadBytes) : options.longs === Number ? new $util.LongBits(message.totalReadBytes.low >>> 0, message.totalReadBytes.high >>> 0).toNumber(true) : message.totalReadBytes;
            if (message.totalWriteBytes != null && message.hasOwnProperty("totalWriteBytes"))
                if (typeof message.totalWriteBytes === "number")
                    object.totalWriteBytes = options.longs === String ? String(message.totalWriteBytes) : message.totalWriteBytes;
                else
                    object.totalWriteBytes = options.longs === String ? $util.Long.prototype.toString.call(message.totalWriteBytes) : options.longs === Number ? new $util.LongBits(message.totalWriteBytes.low >>> 0, message.totalWriteBytes.high >>> 0).toNumber(true) : message.totalWriteBytes;
            if (message.totalNetRecvBytes != null && message.hasOwnProperty("totalNetRecvBytes"))
                if (typeof message.totalNetRecvBytes === "number")
                    object.totalNetRecvBytes = options.longs === String ? String(message.totalNetRecvBytes) : message.totalNetRecvBytes;
                else
                    object.totalNetRecvBytes = options.longs === String ? $util.Long.prototype.toString.call(message.totalNetRecvBytes) : options.longs === Number ? new $util.LongBits(message.totalNetRecvBytes.low >>> 0, message.totalNetRecvBytes.high >>> 0).toNumber(true) : message.totalNetRecvBytes;
            if (message.totalNetSentBytes != null && message.hasOwnProperty("totalNetSentBytes"))
                if (typeof message.totalNetSentBytes === "number")
                    object.totalNetSentBytes = options.longs === String ? String(message.totalNetSentBytes) : message.totalNetSentBytes;
                else
                    object.totalNetSentBytes = options.longs === String ? $util.Long.prototype.toString.call(message.totalNetSentBytes) : options.longs === Number ? new $util.LongBits(message.totalNetSentBytes.low >>> 0, message.totalNetSentBytes.high >>> 0).toNumber(true) : message.totalNetSentBytes;
            if (message.networks && message.networks.length) {
                object.networks = [];
                for (var j = 0; j < message.networks.length; ++j)
                    object.networks[j] = $root.pb.NetworkInterface.toObject(message.networks[j], options);
            }
            if (message.disks && message.disks.length) {
                object.disks = [];
                for (var j = 0; j < message.disks.length; ++j)
                    object.disks[j] = $root.pb.DiskDevice.toObject(message.disks[j], options);
            }
            return object;
        };

        /**
         * Converts this IOInfo to JSON.
         * @function toJSON
         * @memberof pb.IOInfo
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        IOInfo.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for IOInfo
         * @function getTypeUrl
         * @memberof pb.IOInfo
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        IOInfo.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.IOInfo";
        };

        return IOInfo;
    })();

    pb.FaultInfo = (function() {

        /**
         * Properties of a FaultInfo.
         * @memberof pb
         * @interface IFaultInfo
         * @property {number|Long|null} [pageFaults] FaultInfo pageFaults
         * @property {number|Long|null} [majorFaults] FaultInfo majorFaults
         * @property {number|Long|null} [minorFaults] FaultInfo minorFaults
         * @property {number|null} [pageFaultRate] FaultInfo pageFaultRate
         * @property {number|null} [majorFaultRate] FaultInfo majorFaultRate
         * @property {number|null} [minorFaultRate] FaultInfo minorFaultRate
         * @property {number|Long|null} [swapIn] FaultInfo swapIn
         * @property {number|Long|null} [swapOut] FaultInfo swapOut
         * @property {number|null} [swapInRate] FaultInfo swapInRate
         * @property {number|null} [swapOutRate] FaultInfo swapOutRate
         */

        /**
         * Constructs a new FaultInfo.
         * @memberof pb
         * @classdesc Represents a FaultInfo.
         * @implements IFaultInfo
         * @constructor
         * @param {pb.IFaultInfo=} [properties] Properties to set
         */
        function FaultInfo(properties) {
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * FaultInfo pageFaults.
         * @member {number|Long} pageFaults
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.pageFaults = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * FaultInfo majorFaults.
         * @member {number|Long} majorFaults
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.majorFaults = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * FaultInfo minorFaults.
         * @member {number|Long} minorFaults
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.minorFaults = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * FaultInfo pageFaultRate.
         * @member {number} pageFaultRate
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.pageFaultRate = 0;

        /**
         * FaultInfo majorFaultRate.
         * @member {number} majorFaultRate
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.majorFaultRate = 0;

        /**
         * FaultInfo minorFaultRate.
         * @member {number} minorFaultRate
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.minorFaultRate = 0;

        /**
         * FaultInfo swapIn.
         * @member {number|Long} swapIn
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.swapIn = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * FaultInfo swapOut.
         * @member {number|Long} swapOut
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.swapOut = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * FaultInfo swapInRate.
         * @member {number} swapInRate
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.swapInRate = 0;

        /**
         * FaultInfo swapOutRate.
         * @member {number} swapOutRate
         * @memberof pb.FaultInfo
         * @instance
         */
        FaultInfo.prototype.swapOutRate = 0;

        /**
         * Creates a new FaultInfo instance using the specified properties.
         * @function create
         * @memberof pb.FaultInfo
         * @static
         * @param {pb.IFaultInfo=} [properties] Properties to set
         * @returns {pb.FaultInfo} FaultInfo instance
         */
        FaultInfo.create = function create(properties) {
            return new FaultInfo(properties);
        };

        /**
         * Encodes the specified FaultInfo message. Does not implicitly {@link pb.FaultInfo.verify|verify} messages.
         * @function encode
         * @memberof pb.FaultInfo
         * @static
         * @param {pb.IFaultInfo} message FaultInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        FaultInfo.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pageFaults != null && Object.hasOwnProperty.call(message, "pageFaults"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.pageFaults);
            if (message.majorFaults != null && Object.hasOwnProperty.call(message, "majorFaults"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.majorFaults);
            if (message.minorFaults != null && Object.hasOwnProperty.call(message, "minorFaults"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.minorFaults);
            if (message.pageFaultRate != null && Object.hasOwnProperty.call(message, "pageFaultRate"))
                writer.uint32(/* id 4, wireType 1 =*/33).double(message.pageFaultRate);
            if (message.majorFaultRate != null && Object.hasOwnProperty.call(message, "majorFaultRate"))
                writer.uint32(/* id 5, wireType 1 =*/41).double(message.majorFaultRate);
            if (message.minorFaultRate != null && Object.hasOwnProperty.call(message, "minorFaultRate"))
                writer.uint32(/* id 6, wireType 1 =*/49).double(message.minorFaultRate);
            if (message.swapIn != null && Object.hasOwnProperty.call(message, "swapIn"))
                writer.uint32(/* id 7, wireType 0 =*/56).uint64(message.swapIn);
            if (message.swapOut != null && Object.hasOwnProperty.call(message, "swapOut"))
                writer.uint32(/* id 8, wireType 0 =*/64).uint64(message.swapOut);
            if (message.swapInRate != null && Object.hasOwnProperty.call(message, "swapInRate"))
                writer.uint32(/* id 9, wireType 1 =*/73).double(message.swapInRate);
            if (message.swapOutRate != null && Object.hasOwnProperty.call(message, "swapOutRate"))
                writer.uint32(/* id 10, wireType 1 =*/81).double(message.swapOutRate);
            return writer;
        };

        /**
         * Encodes the specified FaultInfo message, length delimited. Does not implicitly {@link pb.FaultInfo.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.FaultInfo
         * @static
         * @param {pb.IFaultInfo} message FaultInfo message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        FaultInfo.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a FaultInfo message from the specified reader or buffer.
         * @function decode
         * @memberof pb.FaultInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.FaultInfo} FaultInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        FaultInfo.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.FaultInfo();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pageFaults = reader.uint64();
                        break;
                    }
                case 2: {
                        message.majorFaults = reader.uint64();
                        break;
                    }
                case 3: {
                        message.minorFaults = reader.uint64();
                        break;
                    }
                case 4: {
                        message.pageFaultRate = reader.double();
                        break;
                    }
                case 5: {
                        message.majorFaultRate = reader.double();
                        break;
                    }
                case 6: {
                        message.minorFaultRate = reader.double();
                        break;
                    }
                case 7: {
                        message.swapIn = reader.uint64();
                        break;
                    }
                case 8: {
                        message.swapOut = reader.uint64();
                        break;
                    }
                case 9: {
                        message.swapInRate = reader.double();
                        break;
                    }
                case 10: {
                        message.swapOutRate = reader.double();
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
         * Decodes a FaultInfo message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.FaultInfo
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.FaultInfo} FaultInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        FaultInfo.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a FaultInfo message.
         * @function verify
         * @memberof pb.FaultInfo
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        FaultInfo.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pageFaults != null && message.hasOwnProperty("pageFaults"))
                if (!$util.isInteger(message.pageFaults) && !(message.pageFaults && $util.isInteger(message.pageFaults.low) && $util.isInteger(message.pageFaults.high)))
                    return "pageFaults: integer|Long expected";
            if (message.majorFaults != null && message.hasOwnProperty("majorFaults"))
                if (!$util.isInteger(message.majorFaults) && !(message.majorFaults && $util.isInteger(message.majorFaults.low) && $util.isInteger(message.majorFaults.high)))
                    return "majorFaults: integer|Long expected";
            if (message.minorFaults != null && message.hasOwnProperty("minorFaults"))
                if (!$util.isInteger(message.minorFaults) && !(message.minorFaults && $util.isInteger(message.minorFaults.low) && $util.isInteger(message.minorFaults.high)))
                    return "minorFaults: integer|Long expected";
            if (message.pageFaultRate != null && message.hasOwnProperty("pageFaultRate"))
                if (typeof message.pageFaultRate !== "number")
                    return "pageFaultRate: number expected";
            if (message.majorFaultRate != null && message.hasOwnProperty("majorFaultRate"))
                if (typeof message.majorFaultRate !== "number")
                    return "majorFaultRate: number expected";
            if (message.minorFaultRate != null && message.hasOwnProperty("minorFaultRate"))
                if (typeof message.minorFaultRate !== "number")
                    return "minorFaultRate: number expected";
            if (message.swapIn != null && message.hasOwnProperty("swapIn"))
                if (!$util.isInteger(message.swapIn) && !(message.swapIn && $util.isInteger(message.swapIn.low) && $util.isInteger(message.swapIn.high)))
                    return "swapIn: integer|Long expected";
            if (message.swapOut != null && message.hasOwnProperty("swapOut"))
                if (!$util.isInteger(message.swapOut) && !(message.swapOut && $util.isInteger(message.swapOut.low) && $util.isInteger(message.swapOut.high)))
                    return "swapOut: integer|Long expected";
            if (message.swapInRate != null && message.hasOwnProperty("swapInRate"))
                if (typeof message.swapInRate !== "number")
                    return "swapInRate: number expected";
            if (message.swapOutRate != null && message.hasOwnProperty("swapOutRate"))
                if (typeof message.swapOutRate !== "number")
                    return "swapOutRate: number expected";
            return null;
        };

        /**
         * Creates a FaultInfo message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.FaultInfo
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.FaultInfo} FaultInfo
         */
        FaultInfo.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.FaultInfo)
                return object;
            var message = new $root.pb.FaultInfo();
            if (object.pageFaults != null)
                if ($util.Long)
                    (message.pageFaults = $util.Long.fromValue(object.pageFaults)).unsigned = true;
                else if (typeof object.pageFaults === "string")
                    message.pageFaults = parseInt(object.pageFaults, 10);
                else if (typeof object.pageFaults === "number")
                    message.pageFaults = object.pageFaults;
                else if (typeof object.pageFaults === "object")
                    message.pageFaults = new $util.LongBits(object.pageFaults.low >>> 0, object.pageFaults.high >>> 0).toNumber(true);
            if (object.majorFaults != null)
                if ($util.Long)
                    (message.majorFaults = $util.Long.fromValue(object.majorFaults)).unsigned = true;
                else if (typeof object.majorFaults === "string")
                    message.majorFaults = parseInt(object.majorFaults, 10);
                else if (typeof object.majorFaults === "number")
                    message.majorFaults = object.majorFaults;
                else if (typeof object.majorFaults === "object")
                    message.majorFaults = new $util.LongBits(object.majorFaults.low >>> 0, object.majorFaults.high >>> 0).toNumber(true);
            if (object.minorFaults != null)
                if ($util.Long)
                    (message.minorFaults = $util.Long.fromValue(object.minorFaults)).unsigned = true;
                else if (typeof object.minorFaults === "string")
                    message.minorFaults = parseInt(object.minorFaults, 10);
                else if (typeof object.minorFaults === "number")
                    message.minorFaults = object.minorFaults;
                else if (typeof object.minorFaults === "object")
                    message.minorFaults = new $util.LongBits(object.minorFaults.low >>> 0, object.minorFaults.high >>> 0).toNumber(true);
            if (object.pageFaultRate != null)
                message.pageFaultRate = Number(object.pageFaultRate);
            if (object.majorFaultRate != null)
                message.majorFaultRate = Number(object.majorFaultRate);
            if (object.minorFaultRate != null)
                message.minorFaultRate = Number(object.minorFaultRate);
            if (object.swapIn != null)
                if ($util.Long)
                    (message.swapIn = $util.Long.fromValue(object.swapIn)).unsigned = true;
                else if (typeof object.swapIn === "string")
                    message.swapIn = parseInt(object.swapIn, 10);
                else if (typeof object.swapIn === "number")
                    message.swapIn = object.swapIn;
                else if (typeof object.swapIn === "object")
                    message.swapIn = new $util.LongBits(object.swapIn.low >>> 0, object.swapIn.high >>> 0).toNumber(true);
            if (object.swapOut != null)
                if ($util.Long)
                    (message.swapOut = $util.Long.fromValue(object.swapOut)).unsigned = true;
                else if (typeof object.swapOut === "string")
                    message.swapOut = parseInt(object.swapOut, 10);
                else if (typeof object.swapOut === "number")
                    message.swapOut = object.swapOut;
                else if (typeof object.swapOut === "object")
                    message.swapOut = new $util.LongBits(object.swapOut.low >>> 0, object.swapOut.high >>> 0).toNumber(true);
            if (object.swapInRate != null)
                message.swapInRate = Number(object.swapInRate);
            if (object.swapOutRate != null)
                message.swapOutRate = Number(object.swapOutRate);
            return message;
        };

        /**
         * Creates a plain object from a FaultInfo message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.FaultInfo
         * @static
         * @param {pb.FaultInfo} message FaultInfo
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        FaultInfo.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.defaults) {
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.pageFaults = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.pageFaults = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.majorFaults = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.majorFaults = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.minorFaults = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.minorFaults = options.longs === String ? "0" : 0;
                object.pageFaultRate = 0;
                object.majorFaultRate = 0;
                object.minorFaultRate = 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.swapIn = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.swapIn = options.longs === String ? "0" : 0;
                if ($util.Long) {
                    var long = new $util.Long(0, 0, true);
                    object.swapOut = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
                } else
                    object.swapOut = options.longs === String ? "0" : 0;
                object.swapInRate = 0;
                object.swapOutRate = 0;
            }
            if (message.pageFaults != null && message.hasOwnProperty("pageFaults"))
                if (typeof message.pageFaults === "number")
                    object.pageFaults = options.longs === String ? String(message.pageFaults) : message.pageFaults;
                else
                    object.pageFaults = options.longs === String ? $util.Long.prototype.toString.call(message.pageFaults) : options.longs === Number ? new $util.LongBits(message.pageFaults.low >>> 0, message.pageFaults.high >>> 0).toNumber(true) : message.pageFaults;
            if (message.majorFaults != null && message.hasOwnProperty("majorFaults"))
                if (typeof message.majorFaults === "number")
                    object.majorFaults = options.longs === String ? String(message.majorFaults) : message.majorFaults;
                else
                    object.majorFaults = options.longs === String ? $util.Long.prototype.toString.call(message.majorFaults) : options.longs === Number ? new $util.LongBits(message.majorFaults.low >>> 0, message.majorFaults.high >>> 0).toNumber(true) : message.majorFaults;
            if (message.minorFaults != null && message.hasOwnProperty("minorFaults"))
                if (typeof message.minorFaults === "number")
                    object.minorFaults = options.longs === String ? String(message.minorFaults) : message.minorFaults;
                else
                    object.minorFaults = options.longs === String ? $util.Long.prototype.toString.call(message.minorFaults) : options.longs === Number ? new $util.LongBits(message.minorFaults.low >>> 0, message.minorFaults.high >>> 0).toNumber(true) : message.minorFaults;
            if (message.pageFaultRate != null && message.hasOwnProperty("pageFaultRate"))
                object.pageFaultRate = options.json && !isFinite(message.pageFaultRate) ? String(message.pageFaultRate) : message.pageFaultRate;
            if (message.majorFaultRate != null && message.hasOwnProperty("majorFaultRate"))
                object.majorFaultRate = options.json && !isFinite(message.majorFaultRate) ? String(message.majorFaultRate) : message.majorFaultRate;
            if (message.minorFaultRate != null && message.hasOwnProperty("minorFaultRate"))
                object.minorFaultRate = options.json && !isFinite(message.minorFaultRate) ? String(message.minorFaultRate) : message.minorFaultRate;
            if (message.swapIn != null && message.hasOwnProperty("swapIn"))
                if (typeof message.swapIn === "number")
                    object.swapIn = options.longs === String ? String(message.swapIn) : message.swapIn;
                else
                    object.swapIn = options.longs === String ? $util.Long.prototype.toString.call(message.swapIn) : options.longs === Number ? new $util.LongBits(message.swapIn.low >>> 0, message.swapIn.high >>> 0).toNumber(true) : message.swapIn;
            if (message.swapOut != null && message.hasOwnProperty("swapOut"))
                if (typeof message.swapOut === "number")
                    object.swapOut = options.longs === String ? String(message.swapOut) : message.swapOut;
                else
                    object.swapOut = options.longs === String ? $util.Long.prototype.toString.call(message.swapOut) : options.longs === Number ? new $util.LongBits(message.swapOut.low >>> 0, message.swapOut.high >>> 0).toNumber(true) : message.swapOut;
            if (message.swapInRate != null && message.hasOwnProperty("swapInRate"))
                object.swapInRate = options.json && !isFinite(message.swapInRate) ? String(message.swapInRate) : message.swapInRate;
            if (message.swapOutRate != null && message.hasOwnProperty("swapOutRate"))
                object.swapOutRate = options.json && !isFinite(message.swapOutRate) ? String(message.swapOutRate) : message.swapOutRate;
            return object;
        };

        /**
         * Converts this FaultInfo to JSON.
         * @function toJSON
         * @memberof pb.FaultInfo
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        FaultInfo.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for FaultInfo
         * @function getTypeUrl
         * @memberof pb.FaultInfo
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        FaultInfo.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.FaultInfo";
        };

        return FaultInfo;
    })();

    pb.SystemStats = (function() {

        /**
         * Properties of a SystemStats.
         * @memberof pb
         * @interface ISystemStats
         * @property {Array.<pb.IProcess>|null} [processes] SystemStats processes
         * @property {Array.<pb.IGPUStatus>|null} [gpus] SystemStats gpus
         * @property {pb.ICPUInfo|null} [cpu] SystemStats cpu
         * @property {pb.IMemoryInfo|null} [memory] SystemStats memory
         * @property {pb.IIOInfo|null} [io] SystemStats io
         * @property {pb.IFaultInfo|null} [faults] SystemStats faults
         */

        /**
         * Constructs a new SystemStats.
         * @memberof pb
         * @classdesc Represents a SystemStats.
         * @implements ISystemStats
         * @constructor
         * @param {pb.ISystemStats=} [properties] Properties to set
         */
        function SystemStats(properties) {
            this.processes = [];
            this.gpus = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * SystemStats processes.
         * @member {Array.<pb.IProcess>} processes
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.processes = $util.emptyArray;

        /**
         * SystemStats gpus.
         * @member {Array.<pb.IGPUStatus>} gpus
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.gpus = $util.emptyArray;

        /**
         * SystemStats cpu.
         * @member {pb.ICPUInfo|null|undefined} cpu
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.cpu = null;

        /**
         * SystemStats memory.
         * @member {pb.IMemoryInfo|null|undefined} memory
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.memory = null;

        /**
         * SystemStats io.
         * @member {pb.IIOInfo|null|undefined} io
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.io = null;

        /**
         * SystemStats faults.
         * @member {pb.IFaultInfo|null|undefined} faults
         * @memberof pb.SystemStats
         * @instance
         */
        SystemStats.prototype.faults = null;

        /**
         * Creates a new SystemStats instance using the specified properties.
         * @function create
         * @memberof pb.SystemStats
         * @static
         * @param {pb.ISystemStats=} [properties] Properties to set
         * @returns {pb.SystemStats} SystemStats instance
         */
        SystemStats.create = function create(properties) {
            return new SystemStats(properties);
        };

        /**
         * Encodes the specified SystemStats message. Does not implicitly {@link pb.SystemStats.verify|verify} messages.
         * @function encode
         * @memberof pb.SystemStats
         * @static
         * @param {pb.ISystemStats} message SystemStats message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        SystemStats.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.processes != null && message.processes.length)
                for (var i = 0; i < message.processes.length; ++i)
                    $root.pb.Process.encode(message.processes[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.gpus != null && message.gpus.length)
                for (var i = 0; i < message.gpus.length; ++i)
                    $root.pb.GPUStatus.encode(message.gpus[i], writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.cpu != null && Object.hasOwnProperty.call(message, "cpu"))
                $root.pb.CPUInfo.encode(message.cpu, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            if (message.memory != null && Object.hasOwnProperty.call(message, "memory"))
                $root.pb.MemoryInfo.encode(message.memory, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            if (message.io != null && Object.hasOwnProperty.call(message, "io"))
                $root.pb.IOInfo.encode(message.io, writer.uint32(/* id 5, wireType 2 =*/42).fork()).ldelim();
            if (message.faults != null && Object.hasOwnProperty.call(message, "faults"))
                $root.pb.FaultInfo.encode(message.faults, writer.uint32(/* id 6, wireType 2 =*/50).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified SystemStats message, length delimited. Does not implicitly {@link pb.SystemStats.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.SystemStats
         * @static
         * @param {pb.ISystemStats} message SystemStats message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        SystemStats.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a SystemStats message from the specified reader or buffer.
         * @function decode
         * @memberof pb.SystemStats
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.SystemStats} SystemStats
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        SystemStats.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.SystemStats();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.processes && message.processes.length))
                            message.processes = [];
                        message.processes.push($root.pb.Process.decode(reader, reader.uint32()));
                        break;
                    }
                case 2: {
                        if (!(message.gpus && message.gpus.length))
                            message.gpus = [];
                        message.gpus.push($root.pb.GPUStatus.decode(reader, reader.uint32()));
                        break;
                    }
                case 3: {
                        message.cpu = $root.pb.CPUInfo.decode(reader, reader.uint32());
                        break;
                    }
                case 4: {
                        message.memory = $root.pb.MemoryInfo.decode(reader, reader.uint32());
                        break;
                    }
                case 5: {
                        message.io = $root.pb.IOInfo.decode(reader, reader.uint32());
                        break;
                    }
                case 6: {
                        message.faults = $root.pb.FaultInfo.decode(reader, reader.uint32());
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
         * Decodes a SystemStats message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.SystemStats
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.SystemStats} SystemStats
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        SystemStats.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a SystemStats message.
         * @function verify
         * @memberof pb.SystemStats
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        SystemStats.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.processes != null && message.hasOwnProperty("processes")) {
                if (!Array.isArray(message.processes))
                    return "processes: array expected";
                for (var i = 0; i < message.processes.length; ++i) {
                    var error = $root.pb.Process.verify(message.processes[i]);
                    if (error)
                        return "processes." + error;
                }
            }
            if (message.gpus != null && message.hasOwnProperty("gpus")) {
                if (!Array.isArray(message.gpus))
                    return "gpus: array expected";
                for (var i = 0; i < message.gpus.length; ++i) {
                    var error = $root.pb.GPUStatus.verify(message.gpus[i]);
                    if (error)
                        return "gpus." + error;
                }
            }
            if (message.cpu != null && message.hasOwnProperty("cpu")) {
                var error = $root.pb.CPUInfo.verify(message.cpu);
                if (error)
                    return "cpu." + error;
            }
            if (message.memory != null && message.hasOwnProperty("memory")) {
                var error = $root.pb.MemoryInfo.verify(message.memory);
                if (error)
                    return "memory." + error;
            }
            if (message.io != null && message.hasOwnProperty("io")) {
                var error = $root.pb.IOInfo.verify(message.io);
                if (error)
                    return "io." + error;
            }
            if (message.faults != null && message.hasOwnProperty("faults")) {
                var error = $root.pb.FaultInfo.verify(message.faults);
                if (error)
                    return "faults." + error;
            }
            return null;
        };

        /**
         * Creates a SystemStats message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.SystemStats
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.SystemStats} SystemStats
         */
        SystemStats.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.SystemStats)
                return object;
            var message = new $root.pb.SystemStats();
            if (object.processes) {
                if (!Array.isArray(object.processes))
                    throw TypeError(".pb.SystemStats.processes: array expected");
                message.processes = [];
                for (var i = 0; i < object.processes.length; ++i) {
                    if (typeof object.processes[i] !== "object")
                        throw TypeError(".pb.SystemStats.processes: object expected");
                    message.processes[i] = $root.pb.Process.fromObject(object.processes[i]);
                }
            }
            if (object.gpus) {
                if (!Array.isArray(object.gpus))
                    throw TypeError(".pb.SystemStats.gpus: array expected");
                message.gpus = [];
                for (var i = 0; i < object.gpus.length; ++i) {
                    if (typeof object.gpus[i] !== "object")
                        throw TypeError(".pb.SystemStats.gpus: object expected");
                    message.gpus[i] = $root.pb.GPUStatus.fromObject(object.gpus[i]);
                }
            }
            if (object.cpu != null) {
                if (typeof object.cpu !== "object")
                    throw TypeError(".pb.SystemStats.cpu: object expected");
                message.cpu = $root.pb.CPUInfo.fromObject(object.cpu);
            }
            if (object.memory != null) {
                if (typeof object.memory !== "object")
                    throw TypeError(".pb.SystemStats.memory: object expected");
                message.memory = $root.pb.MemoryInfo.fromObject(object.memory);
            }
            if (object.io != null) {
                if (typeof object.io !== "object")
                    throw TypeError(".pb.SystemStats.io: object expected");
                message.io = $root.pb.IOInfo.fromObject(object.io);
            }
            if (object.faults != null) {
                if (typeof object.faults !== "object")
                    throw TypeError(".pb.SystemStats.faults: object expected");
                message.faults = $root.pb.FaultInfo.fromObject(object.faults);
            }
            return message;
        };

        /**
         * Creates a plain object from a SystemStats message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.SystemStats
         * @static
         * @param {pb.SystemStats} message SystemStats
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        SystemStats.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults) {
                object.processes = [];
                object.gpus = [];
            }
            if (options.defaults) {
                object.cpu = null;
                object.memory = null;
                object.io = null;
                object.faults = null;
            }
            if (message.processes && message.processes.length) {
                object.processes = [];
                for (var j = 0; j < message.processes.length; ++j)
                    object.processes[j] = $root.pb.Process.toObject(message.processes[j], options);
            }
            if (message.gpus && message.gpus.length) {
                object.gpus = [];
                for (var j = 0; j < message.gpus.length; ++j)
                    object.gpus[j] = $root.pb.GPUStatus.toObject(message.gpus[j], options);
            }
            if (message.cpu != null && message.hasOwnProperty("cpu"))
                object.cpu = $root.pb.CPUInfo.toObject(message.cpu, options);
            if (message.memory != null && message.hasOwnProperty("memory"))
                object.memory = $root.pb.MemoryInfo.toObject(message.memory, options);
            if (message.io != null && message.hasOwnProperty("io"))
                object.io = $root.pb.IOInfo.toObject(message.io, options);
            if (message.faults != null && message.hasOwnProperty("faults"))
                object.faults = $root.pb.FaultInfo.toObject(message.faults, options);
            return object;
        };

        /**
         * Converts this SystemStats to JSON.
         * @function toJSON
         * @memberof pb.SystemStats
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        SystemStats.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for SystemStats
         * @function getTypeUrl
         * @memberof pb.SystemStats
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        SystemStats.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.SystemStats";
        };

        return SystemStats;
    })();

    pb.WrapperRequest = (function() {

        /**
         * Properties of a WrapperRequest.
         * @memberof pb
         * @interface IWrapperRequest
         * @property {number|null} [pid] WrapperRequest pid
         * @property {string|null} [comm] WrapperRequest comm
         * @property {Array.<string>|null} [args] WrapperRequest args
         * @property {string|null} [user] WrapperRequest user
         */

        /**
         * Constructs a new WrapperRequest.
         * @memberof pb
         * @classdesc Represents a WrapperRequest.
         * @implements IWrapperRequest
         * @constructor
         * @param {pb.IWrapperRequest=} [properties] Properties to set
         */
        function WrapperRequest(properties) {
            this.args = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * WrapperRequest pid.
         * @member {number} pid
         * @memberof pb.WrapperRequest
         * @instance
         */
        WrapperRequest.prototype.pid = 0;

        /**
         * WrapperRequest comm.
         * @member {string} comm
         * @memberof pb.WrapperRequest
         * @instance
         */
        WrapperRequest.prototype.comm = "";

        /**
         * WrapperRequest args.
         * @member {Array.<string>} args
         * @memberof pb.WrapperRequest
         * @instance
         */
        WrapperRequest.prototype.args = $util.emptyArray;

        /**
         * WrapperRequest user.
         * @member {string} user
         * @memberof pb.WrapperRequest
         * @instance
         */
        WrapperRequest.prototype.user = "";

        /**
         * Creates a new WrapperRequest instance using the specified properties.
         * @function create
         * @memberof pb.WrapperRequest
         * @static
         * @param {pb.IWrapperRequest=} [properties] Properties to set
         * @returns {pb.WrapperRequest} WrapperRequest instance
         */
        WrapperRequest.create = function create(properties) {
            return new WrapperRequest(properties);
        };

        /**
         * Encodes the specified WrapperRequest message. Does not implicitly {@link pb.WrapperRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.WrapperRequest
         * @static
         * @param {pb.IWrapperRequest} message WrapperRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        WrapperRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            if (message.comm != null && Object.hasOwnProperty.call(message, "comm"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.comm);
            if (message.args != null && message.args.length)
                for (var i = 0; i < message.args.length; ++i)
                    writer.uint32(/* id 3, wireType 2 =*/26).string(message.args[i]);
            if (message.user != null && Object.hasOwnProperty.call(message, "user"))
                writer.uint32(/* id 4, wireType 2 =*/34).string(message.user);
            return writer;
        };

        /**
         * Encodes the specified WrapperRequest message, length delimited. Does not implicitly {@link pb.WrapperRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.WrapperRequest
         * @static
         * @param {pb.IWrapperRequest} message WrapperRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        WrapperRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a WrapperRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.WrapperRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.WrapperRequest} WrapperRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        WrapperRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.WrapperRequest();
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
                        message.comm = reader.string();
                        break;
                    }
                case 3: {
                        if (!(message.args && message.args.length))
                            message.args = [];
                        message.args.push(reader.string());
                        break;
                    }
                case 4: {
                        message.user = reader.string();
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
         * Decodes a WrapperRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.WrapperRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.WrapperRequest} WrapperRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        WrapperRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a WrapperRequest message.
         * @function verify
         * @memberof pb.WrapperRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        WrapperRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            if (message.comm != null && message.hasOwnProperty("comm"))
                if (!$util.isString(message.comm))
                    return "comm: string expected";
            if (message.args != null && message.hasOwnProperty("args")) {
                if (!Array.isArray(message.args))
                    return "args: array expected";
                for (var i = 0; i < message.args.length; ++i)
                    if (!$util.isString(message.args[i]))
                        return "args: string[] expected";
            }
            if (message.user != null && message.hasOwnProperty("user"))
                if (!$util.isString(message.user))
                    return "user: string expected";
            return null;
        };

        /**
         * Creates a WrapperRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.WrapperRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.WrapperRequest} WrapperRequest
         */
        WrapperRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.WrapperRequest)
                return object;
            var message = new $root.pb.WrapperRequest();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            if (object.comm != null)
                message.comm = String(object.comm);
            if (object.args) {
                if (!Array.isArray(object.args))
                    throw TypeError(".pb.WrapperRequest.args: array expected");
                message.args = [];
                for (var i = 0; i < object.args.length; ++i)
                    message.args[i] = String(object.args[i]);
            }
            if (object.user != null)
                message.user = String(object.user);
            return message;
        };

        /**
         * Creates a plain object from a WrapperRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.WrapperRequest
         * @static
         * @param {pb.WrapperRequest} message WrapperRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        WrapperRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults)
                object.args = [];
            if (options.defaults) {
                object.pid = 0;
                object.comm = "";
                object.user = "";
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.comm != null && message.hasOwnProperty("comm"))
                object.comm = message.comm;
            if (message.args && message.args.length) {
                object.args = [];
                for (var j = 0; j < message.args.length; ++j)
                    object.args[j] = message.args[j];
            }
            if (message.user != null && message.hasOwnProperty("user"))
                object.user = message.user;
            return object;
        };

        /**
         * Converts this WrapperRequest to JSON.
         * @function toJSON
         * @memberof pb.WrapperRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        WrapperRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for WrapperRequest
         * @function getTypeUrl
         * @memberof pb.WrapperRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        WrapperRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.WrapperRequest";
        };

        return WrapperRequest;
    })();

    pb.WrapperResponse = (function() {

        /**
         * Properties of a WrapperResponse.
         * @memberof pb
         * @interface IWrapperResponse
         * @property {pb.WrapperResponse.Action|null} [action] WrapperResponse action
         * @property {string|null} [message] WrapperResponse message
         * @property {Array.<string>|null} [rewrittenArgs] WrapperResponse rewrittenArgs
         */

        /**
         * Constructs a new WrapperResponse.
         * @memberof pb
         * @classdesc Represents a WrapperResponse.
         * @implements IWrapperResponse
         * @constructor
         * @param {pb.IWrapperResponse=} [properties] Properties to set
         */
        function WrapperResponse(properties) {
            this.rewrittenArgs = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * WrapperResponse action.
         * @member {pb.WrapperResponse.Action} action
         * @memberof pb.WrapperResponse
         * @instance
         */
        WrapperResponse.prototype.action = 0;

        /**
         * WrapperResponse message.
         * @member {string} message
         * @memberof pb.WrapperResponse
         * @instance
         */
        WrapperResponse.prototype.message = "";

        /**
         * WrapperResponse rewrittenArgs.
         * @member {Array.<string>} rewrittenArgs
         * @memberof pb.WrapperResponse
         * @instance
         */
        WrapperResponse.prototype.rewrittenArgs = $util.emptyArray;

        /**
         * Creates a new WrapperResponse instance using the specified properties.
         * @function create
         * @memberof pb.WrapperResponse
         * @static
         * @param {pb.IWrapperResponse=} [properties] Properties to set
         * @returns {pb.WrapperResponse} WrapperResponse instance
         */
        WrapperResponse.create = function create(properties) {
            return new WrapperResponse(properties);
        };

        /**
         * Encodes the specified WrapperResponse message. Does not implicitly {@link pb.WrapperResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.WrapperResponse
         * @static
         * @param {pb.IWrapperResponse} message WrapperResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        WrapperResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.action != null && Object.hasOwnProperty.call(message, "action"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.action);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            if (message.rewrittenArgs != null && message.rewrittenArgs.length)
                for (var i = 0; i < message.rewrittenArgs.length; ++i)
                    writer.uint32(/* id 3, wireType 2 =*/26).string(message.rewrittenArgs[i]);
            return writer;
        };

        /**
         * Encodes the specified WrapperResponse message, length delimited. Does not implicitly {@link pb.WrapperResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.WrapperResponse
         * @static
         * @param {pb.IWrapperResponse} message WrapperResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        WrapperResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a WrapperResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.WrapperResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.WrapperResponse} WrapperResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        WrapperResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.WrapperResponse();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.action = reader.int32();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                case 3: {
                        if (!(message.rewrittenArgs && message.rewrittenArgs.length))
                            message.rewrittenArgs = [];
                        message.rewrittenArgs.push(reader.string());
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
         * Decodes a WrapperResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.WrapperResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.WrapperResponse} WrapperResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        WrapperResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a WrapperResponse message.
         * @function verify
         * @memberof pb.WrapperResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        WrapperResponse.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.action != null && message.hasOwnProperty("action"))
                switch (message.action) {
                default:
                    return "action: enum value expected";
                case 0:
                case 1:
                case 2:
                case 3:
                    break;
                }
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            if (message.rewrittenArgs != null && message.hasOwnProperty("rewrittenArgs")) {
                if (!Array.isArray(message.rewrittenArgs))
                    return "rewrittenArgs: array expected";
                for (var i = 0; i < message.rewrittenArgs.length; ++i)
                    if (!$util.isString(message.rewrittenArgs[i]))
                        return "rewrittenArgs: string[] expected";
            }
            return null;
        };

        /**
         * Creates a WrapperResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.WrapperResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.WrapperResponse} WrapperResponse
         */
        WrapperResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.WrapperResponse)
                return object;
            var message = new $root.pb.WrapperResponse();
            switch (object.action) {
            default:
                if (typeof object.action === "number") {
                    message.action = object.action;
                    break;
                }
                break;
            case "ALLOW":
            case 0:
                message.action = 0;
                break;
            case "BLOCK":
            case 1:
                message.action = 1;
                break;
            case "REWRITE":
            case 2:
                message.action = 2;
                break;
            case "ALERT":
            case 3:
                message.action = 3;
                break;
            }
            if (object.message != null)
                message.message = String(object.message);
            if (object.rewrittenArgs) {
                if (!Array.isArray(object.rewrittenArgs))
                    throw TypeError(".pb.WrapperResponse.rewrittenArgs: array expected");
                message.rewrittenArgs = [];
                for (var i = 0; i < object.rewrittenArgs.length; ++i)
                    message.rewrittenArgs[i] = String(object.rewrittenArgs[i]);
            }
            return message;
        };

        /**
         * Creates a plain object from a WrapperResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.WrapperResponse
         * @static
         * @param {pb.WrapperResponse} message WrapperResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        WrapperResponse.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults)
                object.rewrittenArgs = [];
            if (options.defaults) {
                object.action = options.enums === String ? "ALLOW" : 0;
                object.message = "";
            }
            if (message.action != null && message.hasOwnProperty("action"))
                object.action = options.enums === String ? $root.pb.WrapperResponse.Action[message.action] === undefined ? message.action : $root.pb.WrapperResponse.Action[message.action] : message.action;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            if (message.rewrittenArgs && message.rewrittenArgs.length) {
                object.rewrittenArgs = [];
                for (var j = 0; j < message.rewrittenArgs.length; ++j)
                    object.rewrittenArgs[j] = message.rewrittenArgs[j];
            }
            return object;
        };

        /**
         * Converts this WrapperResponse to JSON.
         * @function toJSON
         * @memberof pb.WrapperResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        WrapperResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for WrapperResponse
         * @function getTypeUrl
         * @memberof pb.WrapperResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        WrapperResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.WrapperResponse";
        };

        /**
         * Action enum.
         * @name pb.WrapperResponse.Action
         * @enum {number}
         * @property {number} ALLOW=0 ALLOW value
         * @property {number} BLOCK=1 BLOCK value
         * @property {number} REWRITE=2 REWRITE value
         * @property {number} ALERT=3 ALERT value
         */
        WrapperResponse.Action = (function() {
            var valuesById = {}, values = Object.create(valuesById);
            values[valuesById[0] = "ALLOW"] = 0;
            values[valuesById[1] = "BLOCK"] = 1;
            values[valuesById[2] = "REWRITE"] = 2;
            values[valuesById[3] = "ALERT"] = 3;
            return values;
        })();

        return WrapperResponse;
    })();

    pb.ProcessList = (function() {

        /**
         * Properties of a ProcessList.
         * @memberof pb
         * @interface IProcessList
         * @property {Array.<pb.IProcess>|null} [processes] ProcessList processes
         */

        /**
         * Constructs a new ProcessList.
         * @memberof pb
         * @classdesc Represents a ProcessList.
         * @implements IProcessList
         * @constructor
         * @param {pb.IProcessList=} [properties] Properties to set
         */
        function ProcessList(properties) {
            this.processes = [];
            if (properties)
                for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ProcessList processes.
         * @member {Array.<pb.IProcess>} processes
         * @memberof pb.ProcessList
         * @instance
         */
        ProcessList.prototype.processes = $util.emptyArray;

        /**
         * Creates a new ProcessList instance using the specified properties.
         * @function create
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList=} [properties] Properties to set
         * @returns {pb.ProcessList} ProcessList instance
         */
        ProcessList.create = function create(properties) {
            return new ProcessList(properties);
        };

        /**
         * Encodes the specified ProcessList message. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @function encode
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList} message ProcessList message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ProcessList.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.processes != null && message.processes.length)
                for (var i = 0; i < message.processes.length; ++i)
                    $root.pb.Process.encode(message.processes[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ProcessList message, length delimited. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList} message ProcessList message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ProcessList.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ProcessList message from the specified reader or buffer.
         * @function decode
         * @memberof pb.ProcessList
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.ProcessList} ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ProcessList.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            var end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.ProcessList();
            while (reader.pos < end) {
                var tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.processes && message.processes.length))
                            message.processes = [];
                        message.processes.push($root.pb.Process.decode(reader, reader.uint32()));
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
         * Decodes a ProcessList message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.ProcessList
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.ProcessList} ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ProcessList.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ProcessList message.
         * @function verify
         * @memberof pb.ProcessList
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ProcessList.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.processes != null && message.hasOwnProperty("processes")) {
                if (!Array.isArray(message.processes))
                    return "processes: array expected";
                for (var i = 0; i < message.processes.length; ++i) {
                    var error = $root.pb.Process.verify(message.processes[i]);
                    if (error)
                        return "processes." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ProcessList message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.ProcessList
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.ProcessList} ProcessList
         */
        ProcessList.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.ProcessList)
                return object;
            var message = new $root.pb.ProcessList();
            if (object.processes) {
                if (!Array.isArray(object.processes))
                    throw TypeError(".pb.ProcessList.processes: array expected");
                message.processes = [];
                for (var i = 0; i < object.processes.length; ++i) {
                    if (typeof object.processes[i] !== "object")
                        throw TypeError(".pb.ProcessList.processes: object expected");
                    message.processes[i] = $root.pb.Process.fromObject(object.processes[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a ProcessList message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.ProcessList
         * @static
         * @param {pb.ProcessList} message ProcessList
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ProcessList.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            var object = {};
            if (options.arrays || options.defaults)
                object.processes = [];
            if (message.processes && message.processes.length) {
                object.processes = [];
                for (var j = 0; j < message.processes.length; ++j)
                    object.processes[j] = $root.pb.Process.toObject(message.processes[j], options);
            }
            return object;
        };

        /**
         * Converts this ProcessList to JSON.
         * @function toJSON
         * @memberof pb.ProcessList
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ProcessList.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ProcessList
         * @function getTypeUrl
         * @memberof pb.ProcessList
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ProcessList.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.ProcessList";
        };

        return ProcessList;
    })();

    return pb;
})();

module.exports = $root;
